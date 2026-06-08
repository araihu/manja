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

  document.addEventListener("DOMContentLoaded", function () {
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
