(function (root, factory) {
  const api = factory();

  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }

  if (root) {
    root.ManjaSchemaExamples = api;
    if (root.document) {
      const run = () => api.hydrateSchemaExamples({
        roots: root.document.querySelectorAll('[data-manja-example]'),
        sampler: root.OpenAPISampler,
        logger: root.console,
      });
      if (root.document.readyState === 'loading') {
        root.document.addEventListener('DOMContentLoaded', run, { once: true });
      } else {
        run();
      }
    }
  }
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  function hydrateSchemaExamples({ roots, sampler, logger } = {}) {
    if (!roots) {
      return;
    }

    Array.from(roots).forEach((root) => {
      if (!root || root.dataset.manjaExampleHydrated === 'true') {
        return;
      }
      root.dataset.manjaExampleHydrated = 'true';

      const code = root.querySelector('.codeblock code') || root.querySelector('.codeblock');
      const payloadScript = root.querySelector('script[type="application/json"]');
      const status = root.querySelector('[data-manja-example-status]');
      if (!code || !payloadScript) {
        setStatus(status, 'Example unavailable');
        return;
      }

      try {
        const payload = JSON.parse(payloadScript.textContent || '{}');
        if (payload.hasExplicitExample) {
          setStatus(status, 'Spec example');
          return;
        }
        if (!payload.schema) {
          setStatus(status, 'Example unavailable');
          return;
        }
        if (!sampler || typeof sampler.sample !== 'function') {
          setStatus(status, 'Example unavailable');
          return;
        }
        const sample = sampler.sample(payload.schema, payload.options || {}, payload.spec);
        setJSONCode(code, JSON.stringify(sample, null, 2));
        setStatus(status, 'Example generated');
      } catch (error) {
        setStatus(status, 'Example unavailable');
        if (logger && typeof logger.debug === 'function') {
          logger.debug('manja schema example generation failed', error);
        }
      }
    });
  }

  function setJSONCode(code, json) {
    code.textContent = json;
    code.innerHTML = highlightJSON(json);
  }

  function highlightJSON(json) {
    const tokenPattern = /("(?:\\u[a-fA-F0-9]{4}|\\[^u]|[^\\"])*"(?:\s*:)?|\b(?:true|false|null)\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|[{}\[\],:])/g;
    let highlighted = '';
    let lastIndex = 0;
    let match;

    while ((match = tokenPattern.exec(json)) !== null) {
      const token = match[0];
      highlighted += escapeHTML(json.slice(lastIndex, match.index));
      highlighted += highlightJSONToken(token);
      lastIndex = match.index + token.length;
    }

    return highlighted + escapeHTML(json.slice(lastIndex));
  }

  function highlightJSONToken(token) {
    if (token.startsWith('"')) {
      const key = token.match(/^(.*?)(\s*:)$/);
      if (key) {
        return `<span class="ch-nt">${escapeHTML(key[1])}</span><span class="ch-p">${escapeHTML(key[2])}</span>`;
      }
      return `<span class="ch-s2">${escapeHTML(token)}</span>`;
    }
    if (token === 'true' || token === 'false' || token === 'null') {
      return `<span class="ch-kc">${token}</span>`;
    }
    if (/^-?\d/.test(token)) {
      const className = token.includes('.') || /e/i.test(token) ? 'ch-mf' : 'ch-mi';
      return `<span class="${className}">${token}</span>`;
    }
    return `<span class="ch-p">${escapeHTML(token)}</span>`;
  }

  function escapeHTML(value) {
    return String(value).replace(/[&<>"']/g, (char) => {
      switch (char) {
        case '&':
          return '&amp;';
        case '<':
          return '&lt;';
        case '>':
          return '&gt;';
        case '"':
          return '&quot;';
        default:
          return '&#39;';
      }
    });
  }

  function setStatus(status, text) {
    if (status) {
      status.textContent = text;
    }
  }

  return { hydrateSchemaExamples, highlightJSON };
});
