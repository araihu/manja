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
    if (!roots || !sampler || typeof sampler.sample !== 'function') {
      return;
    }

    Array.from(roots).forEach((root) => {
      if (!root || root.dataset.manjaExampleHydrated === 'true') {
        return;
      }
      root.dataset.manjaExampleHydrated = 'true';

      const code = root.querySelector('.codeblock');
      const payloadScript = root.querySelector('script[type="application/json"]');
      const status = root.querySelector('[data-manja-example-status]');
      if (!code || !payloadScript) {
        setStatus(status, 'Example unavailable');
        return;
      }

      try {
        const payload = JSON.parse(payloadScript.textContent || '{}');
        if (!payload.schema) {
          setStatus(status, 'Example unavailable');
          return;
        }
        const sample = sampler.sample(payload.schema, payload.options || {}, payload.spec);
        code.textContent = JSON.stringify(sample, null, 2);
        setStatus(status, 'Example generated');
      } catch (error) {
        setStatus(status, 'Example unavailable');
        if (logger && typeof logger.debug === 'function') {
          logger.debug('manja schema example generation failed', error);
        }
      }
    });
  }

  function setStatus(status, text) {
    if (status) {
      status.textContent = text;
    }
  }

  return { hydrateSchemaExamples };
});
