(function () {
  function preferredDark() {
    try {
      var stored = localStorage.getItem("darkMode");
      if (stored !== null) return stored === "true";
    } catch (error) {}
    return window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
  }

  function apply(on) {
    document.documentElement.classList.toggle("dark", on);
    document.querySelectorAll("[data-theme-toggle]").forEach(function (button) {
      button.setAttribute("aria-pressed", String(on));
    });
  }

  function syncCampaignToggleLabels() {
    document.querySelectorAll("[data-campaign-toggle]").forEach(function (button) {
      var label = button.getAttribute("aria-pressed") === "true"
        ? button.dataset.useBaselineLabel
        : button.dataset.useCampaignLabel;
      if (!label) return;
      button.setAttribute("aria-label", label);
      var text = button.querySelector("[data-campaign-toggle-label]");
      if (text) text.textContent = label;
    });
  }

  document.addEventListener("araihu:campaign:applied", syncCampaignToggleLabels);
  document.addEventListener("araihu:campaign:restored", syncCampaignToggleLabels);

  document.addEventListener("DOMContentLoaded", function () {
		syncCampaignToggleLabels();
    apply(preferredDark());
    document.querySelectorAll("[data-theme-toggle]").forEach(function (button) {
      button.addEventListener("click", function () {
        var on = !document.documentElement.classList.contains("dark");
        try {
          localStorage.setItem("darkMode", on ? "true" : "false");
        } catch (error) {}
        apply(on);
      });
    });
  });
})();
