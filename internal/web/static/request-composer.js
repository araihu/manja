(function (root, factory) {
  const api = factory();

  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }

  if (root) {
    root.ManjaRequestComposer = api;
    if (root.document) {
      const run = () => api.hydrateRequestComposers({
        roots: root.document.querySelectorAll('[data-manja-request-composer]'),
        sampler: root.OpenAPISampler,
        logger: root.console,
      });
      if (root.document.readyState === 'loading') {
        root.document.addEventListener('DOMContentLoaded', run, { once: true });
      } else {
        run();
      }
      root.document.addEventListener('htmx:afterSwap', () => run());
    }
  }
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  function hydrateRequestComposers({ roots, sampler, logger } = {}) {
    if (!roots) {
      return;
    }
    Array.from(roots).forEach((root) => hydrateRequestComposer(root, sampler, logger));
  }

  function hydrateRequestComposer(root, sampler, logger) {
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
    hydrateBodyInput(root, bodyInput, payload.body, sampler, logger);

    const render = () => {
      const state = collectState(root, payload, bodyInput);
      setCode(sampleCode, buildCurl(state));
    };

    root.dataset.manjaRequestComposerHydrated = 'true';
    root.querySelectorAll('input, select, textarea').forEach((input) => {
      input.addEventListener('input', render);
      input.addEventListener('change', render);
    });
    render();
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
    if (input.type === 'checkbox') {
      return input.checked ? input.value || 'true' : '';
    }
    return input.value || '';
  }

  function setCode(code, value) {
    code.textContent = value;
  }

  function setStatus(status, text) {
    if (status) {
      status.textContent = text;
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
    buildCurl,
    composeURL,
  };
});
