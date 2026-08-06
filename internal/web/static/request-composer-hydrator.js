(function (root, factory) {
  const api = factory(root && root.ManjaHTTPSnippet, root && root.ManjaHighlight);

  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }

  if (root) {
    root.ManjaRequestComposer = api;
    if (root.document) {
      const run = () => api.hydrateRequestComposers({
        roots: root.document.querySelectorAll('[data-manja-request-composer]'),
        sampler: root.OpenAPISampler,
        snippetGenerator: api.createHTTPSnippetGenerator(root.ManjaHTTPSnippet, root.console),
        syntaxHighlighter: api.createSyntaxHighlighter(root.ManjaHighlight, root.console),
        logger: root.console,
      });
      const hydrateDescriptions = () => api.hydrateCollapsibleRequestDescriptions(root.document);
      const hydrateFieldDescriptions = () => api.hydrateRequestConfigFieldDescriptions(root.document);
      const hydrate = () => {
        run();
        hydrateDescriptions();
        hydrateFieldDescriptions();
      };
      if (root.document.readyState === 'loading') {
        root.document.addEventListener('DOMContentLoaded', hydrate, { once: true });
      } else {
        hydrate();
      }
      root.document.addEventListener('htmx:afterSwap', hydrate);
    }
  }
})(typeof globalThis !== 'undefined' ? globalThis : this, function (snippetLib, highlightLib) {
  function hydrateRequestComposers({ roots, sampler, snippetGenerator, syntaxHighlighter, logger } = {}) {
    if (!roots) {
      return;
    }
    const generator = snippetGenerator || createHTTPSnippetGenerator(snippetLib, logger);
    const highlighter = syntaxHighlighter || createSyntaxHighlighter(highlightLib, logger);
    Array.from(roots).forEach((root) => hydrateRequestComposer(root, sampler, generator, highlighter, logger));
  }

  function hydrateCollapsibleRequestDescriptions(root, { threshold = 220 } = {}) {
    const documentRoot = root && root.ownerDocument ? root.ownerDocument : root;
    if (!documentRoot || typeof documentRoot.querySelectorAll !== 'function') {
      return;
    }
    const descriptions = documentRoot.querySelectorAll(
      '[data-manja-request-config-fields] p.text-xs:not([data-manja-request-body-status])',
    );
    Array.from(descriptions).forEach((description, index) => {
      if (description.dataset.manjaCollapsibleDescription === 'true') {
        return;
      }
      const text = String(description.textContent || '').trim();
      if (text.length <= threshold) {
        return;
      }
      const owner = description.closest('[data-manja-request-config-fields]');
      const ownerID = owner && owner.id ? owner.id : 'request-config-fields';
      let descriptionID = description.id || `${ownerID}-description-${index + 1}`;
      let suffix = index + 1;
      while (documentRoot.getElementById(descriptionID) && documentRoot.getElementById(descriptionID) !== description) {
        suffix += 1;
        descriptionID = `${ownerID}-description-${suffix}`;
      }
      description.id = descriptionID;
      description.dataset.manjaCollapsibleDescription = 'true';
      description.dataset.expanded = 'false';
      const toggle = documentRoot.createElement('button');
      toggle.type = 'button';
      toggle.className = 'manja-description-toggle mt-1 text-xs font-medium text-primary underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary dark:text-primary-dark dark:focus-visible:outline-primary-dark';
      toggle.dataset.manjaDescriptionToggle = 'true';
      toggle.setAttribute('aria-controls', descriptionID);
      toggle.setAttribute('aria-expanded', 'false');
      toggle.textContent = 'Show more';
      toggle.addEventListener('click', () => {
        const expanded = description.dataset.expanded === 'true';
        description.dataset.expanded = String(!expanded);
        toggle.setAttribute('aria-expanded', String(!expanded));
        toggle.textContent = expanded ? 'Show more' : 'Show less';
      });
      description.insertAdjacentElement('afterend', toggle);
    });
  }

  function hydrateRequestConfigFieldDescriptions(root) {
    const documentRoot = root && root.ownerDocument ? root.ownerDocument : root;
    if (!documentRoot || typeof documentRoot.querySelectorAll !== 'function') {
      return;
    }
    const fieldsRoots = documentRoot.querySelectorAll('[data-manja-request-config-fields]');
    Array.from(fieldsRoots).forEach((fieldsRoot) => {
      const fields = fieldsRoot.querySelectorAll('[data-field-path]');
      Array.from(fields).forEach((field) => {
        const control = field.querySelector('input[id], select[id], textarea[id]');
        const description = field.querySelector('p.text-xs:not([data-manja-request-body-status])');
        if (!control || !description || !control.id) {
          return;
        }
        const baseID = description.id || `${control.id}-description`;
        let descriptionID = baseID;
        let suffix = 2;
        while (documentRoot.getElementById(descriptionID) && documentRoot.getElementById(descriptionID) !== description) {
          descriptionID = `${baseID}-${suffix}`;
          suffix += 1;
        }
        description.id = descriptionID;
        const describedBy = new Set(String(control.getAttribute('aria-describedby') || '').split(/\s+/).filter(Boolean));
        describedBy.add(descriptionID);
        control.setAttribute('aria-describedby', Array.from(describedBy).join(' '));
      });
    });
  }

  function hydrateRequestComposer(root, sampler, snippetGenerator, syntaxHighlighter, logger) {
    if (!root || root.dataset.manjaRequestComposerHydrated === 'true') {
      return;
    }
    const payloadScript = root.querySelector('script[id$="-request-composer-payload"][type="application/json"]') ||
      root.querySelector('script[type="application/json"]');
    const sampleCode = root.querySelector('[data-manja-request-sample] .codeblock code') ||
      root.querySelector('[data-manja-request-sample] .codeblock');
    if (!payloadScript || !sampleCode) {
      return;
    }

    let payload = {};
    try {
      payload = JSON.parse(payloadScript.textContent || '{}');
    } catch (error) {
      if (logger && typeof logger.debug === 'function') {
        logger.debug('manja request composer payload failed to parse', error);
      }
      return;
    }

    const bodyInput = root.querySelector('[data-manja-request-body-input]');
    const sampleTarget = root.querySelector('[data-manja-request-sample-target]') ||
      root.querySelector('[name="requestSampleTarget"]');
    const bodyHighlight = root.querySelector('[data-manja-request-body-highlight]');
    hydrateBodyInput(root, bodyInput, payload.body, sampler, logger);
    updateBodyHighlight(bodyInput, bodyHighlight, syntaxHighlighter, logger);
    if (bodyInput && bodyHighlight) {
      bodyInput.addEventListener('scroll', () => syncBodyHighlightScroll(bodyInput, bodyHighlight));
    }

    const render = () => {
      const state = collectState(root, payload, bodyInput);
      const target = selectedSampleTarget(payload, sampleTarget);
      setCode(sampleCode, buildRequestSample(state, target, snippetGenerator, logger), target, syntaxHighlighter, logger);
      updateBodyHighlight(bodyInput, bodyHighlight, syntaxHighlighter, logger);
      setRequestSampleTitle(root, target);
    };

    root.dataset.manjaRequestComposerHydrated = 'true';
    root.querySelectorAll('input, select, textarea').forEach((input) => {
      input.addEventListener('input', render);
      input.addEventListener('change', render);
    });
    const scheduleRender = () => setTimeout(render, 0);
    const targetInput = sampleTargetInput(sampleTarget);
    if (sampleTarget && sampleTarget !== targetInput && typeof sampleTarget.addEventListener === 'function') {
      sampleTarget.addEventListener('click', scheduleRender);
      sampleTarget.addEventListener('keydown', (event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          scheduleRender();
        }
      });
    }
    if (typeof root.addEventListener === 'function') {
      root.addEventListener('manja-request-sample-target-change', (event) => {
        const value = event && event.detail ? event.detail.value : '';
        if (targetInput && value) {
          targetInput.value = value;
        }
        render();
      });
    }
    render();
  }

  function updateBodyHighlight(bodyInput, bodyHighlight, syntaxHighlighter, logger) {
    if (!bodyInput || !bodyHighlight) {
      return;
    }
    const value = bodyInput.value || '';
    const highlighted = highlightCode(value, 'json', syntaxHighlighter, logger);
    bodyHighlight.textContent = value;
    if (highlighted) {
      bodyHighlight.innerHTML = highlighted;
      const editor = bodyInput.closest && bodyInput.closest('[data-manja-request-body-editor]');
      if (editor && editor.dataset) {
        editor.dataset.manjaRequestBodyHighlighted = 'true';
      }
      syncBodyHighlightScroll(bodyInput, bodyHighlight);
    }
  }

  function syncBodyHighlightScroll(bodyInput, bodyHighlight) {
    const scroller = bodyHighlight.parentElement || bodyHighlight;
    if (scroller) {
      scroller.scrollTop = bodyInput.scrollTop || 0;
      scroller.scrollLeft = bodyInput.scrollLeft || 0;
    }
  }

  function hydrateBodyInput(root, bodyInput, bodyPayload, sampler, logger) {
    if (!bodyInput || !bodyPayload || bodyPayload.hasExplicitExample) {
      return;
    }
    const status = root.querySelector('[data-manja-request-body-status]');
    if (!bodyPayload.schema || !sampler || typeof sampler.sample !== 'function') {
      setStatus(status, 'Body example unavailable');
      return;
    }
    try {
      const sample = sampler.sample(bodyPayload.schema, bodyPayload.options || {}, bodyPayload.spec);
      bodyInput.value = JSON.stringify(sample, null, 2);
      setStatus(status, 'Body example generated');
    } catch (error) {
      setStatus(status, 'Body example unavailable');
      if (logger && typeof logger.debug === 'function') {
        logger.debug('manja request body example generation failed', error);
      }
    }
  }

  function collectState(root, payload, bodyInput) {
    const serverVariables = {};
    root.querySelectorAll('[name^="server."]').forEach((input) => {
      serverVariables[input.name.slice('server.'.length)] = inputValue(input);
    });

    const parameters = (payload.parameters || []).map((param) => {
      const input = root.querySelector(`[name="${cssEscape(param.fieldName)}"]`);
      return {
        name: param.name,
        in: param.in,
        value: input ? inputValue(input) : '',
      };
    });

    return {
      method: payload.method || 'GET',
      urlTemplate: payload.urlTemplate || '',
      serverVariables,
      parameters,
      body: bodyInput ? bodyInput.value : '',
      bodyContentType: payload.bodyContentType || '',
    };
  }

  function buildCurl(state) {
    const lines = [`curl --request ${String(state.method || 'GET').toUpperCase()}`];
    const url = composeURL(state);
    if (url) {
      lines.push(`  --url '${shellSingleQuote(url)}'`);
    }

    const headers = [];
    for (const param of state.parameters || []) {
      if (param.in === 'header' && param.value) {
        headers.push([param.name, param.value]);
      }
    }
    const body = String(state.body || '').trim();
    if (body && state.bodyContentType && !headers.some(([name]) => name.toLowerCase() === 'content-type')) {
      headers.push(['content-type', state.bodyContentType]);
    }
    for (const [name, value] of headers) {
      lines.push(`  --header '${shellSingleQuote(`${name}: ${value}`)}'`);
    }
    if (body) {
      lines.push(`  --data '${shellSingleQuote(body)}'`);
    }

    return lines.join(' \\\n');
  }

  function buildRequestSample(state, target, snippetGenerator, logger) {
    if (target && target.target === 'shell' && target.client === 'curl') {
      return buildCurl(state);
    }
    if (!snippetGenerator || !target || !target.target || !target.client) {
      return buildCurl(state);
    }
    try {
      const converted = snippetGenerator({
        request: buildHARRequest(state),
        target: target.target,
        client: target.client,
        state,
      });
      const code = snippetCode(converted);
      if (code) {
        return code.replaceAll('%7B', '{').replaceAll('%7D', '}');
      }
    } catch (error) {
      if (logger && typeof logger.debug === 'function') {
        logger.debug('manja request sample generation failed', error);
      }
    }
    return buildCurl(state);
  }

  function buildHARRequest(state) {
    const request = {
      method: String(state.method || 'GET').toUpperCase(),
      url: composeURL(state),
      headers: requestHeaders(state).map(([name, value]) => ({ name, value })),
    };
    const body = String(state.body || '').trim();
    if (body) {
      request.postData = {
        mimeType: state.bodyContentType || 'text/plain',
        text: body,
      };
    }
    return request;
  }

  function requestHeaders(state) {
    const headers = [];
    for (const param of state.parameters || []) {
      if (param.in === 'header' && param.value) {
        headers.push([param.name, param.value]);
      }
    }
    const body = String(state.body || '').trim();
    if (body && state.bodyContentType && !headers.some(([name]) => name.toLowerCase() === 'content-type')) {
      headers.push(['content-type', state.bodyContentType]);
    }
    return headers;
  }

  function selectedSampleTarget(payload, targetControl) {
    const targets = sampleTargets(payload);
    const selectedValue = inputValue(sampleTargetInput(targetControl)) || (targets[0] && targets[0].value) || 'shell:curl';
    return targets.find((target) => target.value === selectedValue) || targets[0] || {
      value: 'shell:curl',
      label: 'cURL',
      target: 'shell',
      client: 'curl',
      language: 'shell',
    };
  }

  function sampleTargets(payload) {
    if (!Array.isArray(payload.sampleTargets)) {
      return [];
    }
    return payload.sampleTargets
      .filter((target) => target && target.value && target.target && target.client)
      .map((target) => ({
        value: String(target.value),
        label: String(target.label || target.value),
        target: String(target.target),
        client: String(target.client),
        language: String(target.language || target.target),
      }));
  }

  function sampleTargetInput(targetControl) {
    if (!targetControl) {
      return null;
    }
    if (typeof targetControl.value === 'string') {
      return targetControl;
    }
    if (typeof targetControl.querySelector === 'function') {
      return targetControl.querySelector('[name="requestSampleTarget"]');
    }
    return null;
  }

  function snippetCode(converted) {
    if (Array.isArray(converted)) {
      return converted.find((value) => typeof value === 'string' && value.trim()) || '';
    }
    if (typeof converted === 'string') {
      return converted;
    }
    return '';
  }

  function createHTTPSnippetGenerator(lib, logger) {
    if (!lib || typeof lib.HTTPSnippet !== 'function') {
      return null;
    }
    return ({ request, target, client }) => {
      const snippet = new lib.HTTPSnippet(request);
      return snippet.convert(target, client);
    };
  }

  function createSyntaxHighlighter(lib, logger) {
    if (!lib || typeof lib.highlight !== 'function') {
      return null;
    }
    return (code, language) => {
      const normalized = normalizeHighlightLanguage(language);
      if (!normalized) {
        return '';
      }
      if (typeof lib.getLanguage === 'function' && !lib.getLanguage(normalized)) {
        return '';
      }
      const result = lib.highlight(code, {
        language: normalized,
        ignoreIllegals: true,
      });
      return result && typeof result.value === 'string' ? result.value : '';
    };
  }

  function composeURL(state) {
    let url = String(state.urlTemplate || '');
    for (const [name, value] of Object.entries(state.serverVariables || {})) {
      url = replaceTemplateValue(url, name, value);
    }
    for (const param of state.parameters || []) {
      if (param.in === 'path' && param.value) {
        url = replaceTemplateValue(url, param.name, encodeURIComponent(param.value));
      }
    }

    const query = [];
    for (const param of state.parameters || []) {
      if (param.in === 'query' && param.value) {
        query.push([param.name, param.value]);
      }
    }
    return appendQuery(url, query);
  }

  function replaceTemplateValue(input, name, value) {
    if (!value) {
      return input;
    }
    const escapedName = String(name).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    return input.replace(new RegExp(`\\{${escapedName}\\}`, 'g'), value);
  }

  function appendQuery(url, query) {
    if (!query.length) {
      return url;
    }
    const separator = url.includes('?') ? '&' : '?';
    return url + separator + query.map(([name, value]) => `${encodeURIComponent(name)}=${encodeURIComponent(value)}`).join('&');
  }

  function inputValue(input) {
    if (!input) {
      return '';
    }
    if (input.type === 'checkbox') {
      return input.checked ? input.value || 'true' : '';
    }
    return input.value || '';
  }

  function setCode(code, value, target, syntaxHighlighter, logger) {
    code.textContent = value;
    const highlighted = highlightCode(value, target && target.language, syntaxHighlighter, logger);
    if (highlighted) {
      code.innerHTML = highlighted;
    }
  }

  function highlightCode(code, language, syntaxHighlighter, logger) {
    if (!syntaxHighlighter || typeof syntaxHighlighter !== 'function') {
      return '';
    }
    try {
      return syntaxHighlighter(code, language) || '';
    } catch (error) {
      if (logger && typeof logger.debug === 'function') {
        logger.debug('manja request sample highlighting failed', error);
      }
      return '';
    }
  }

  function normalizeHighlightLanguage(language) {
    switch (String(language || '').toLowerCase()) {
      case 'bash':
      case 'shell':
      case 'sh':
        return 'bash';
      case 'csharp':
      case 'cs':
        return 'csharp';
      case 'objc':
      case 'objective-c':
        return 'objectivec';
      case 'http':
      case 'http1':
      case 'http1.1':
        return 'http';
      default:
        return String(language || '').toLowerCase();
    }
  }

  function setStatus(status, text) {
    if (status) {
      status.textContent = text;
    }
  }

  function setRequestSampleTitle(root, target) {
    const label = `Request Sample: ${(target && target.label) || 'Shell / cURL'}`;
    const codeblock = root.querySelector('[data-manja-request-sample] .codeblock');
    const header = codeblock && codeblock.previousElementSibling;
    const title = header && header.querySelector('span');
    if (title) {
      title.textContent = label;
    }
    const copyButton = header && header.querySelector('button[aria-label]');
    if (copyButton) {
      copyButton.setAttribute('aria-label', `Copy ${label} code`);
    }
  }

  function shellSingleQuote(value) {
    return String(value).replace(/'/g, `'\\''`);
  }

  function cssEscape(value) {
    if (typeof CSS !== 'undefined' && CSS.escape) {
      return CSS.escape(value);
    }
    return String(value).replace(/["\\]/g, '\\$&');
  }

  return {
    hydrateRequestComposers,
    hydrateCollapsibleRequestDescriptions,
    hydrateRequestConfigFieldDescriptions,
    createHTTPSnippetGenerator,
    createSyntaxHighlighter,
    highlightCode,
    buildHARRequest,
    buildRequestSample,
    buildCurl,
    composeURL,
  };
});
