(function () {
  "use strict";

  if (window.ManjaCatalogSearchRouter && window.manjaCatalogSearch) return;

  document.addEventListener("change", function (event) {
    var picker = event.target;
    if (!picker || !picker.matches || !picker.matches("[data-publication-picker]")) return;
    var context = picker.closest("[data-publication-controls]");
    if (!context) return;
    var target = Array.from(context.querySelectorAll("[data-publication-target]")).find(function (link) {
      return link.dataset.publicationTarget === picker.value;
    });
    if (target) window.location.assign(target.href);
  });

  // Restore composite picks from their document shell after BFCache restores
  // the selection that changed immediately before navigating away.
  window.addEventListener("pageshow", function (event) {
    if (!event.persisted) return;
    document.querySelectorAll("[data-publication-picker]").forEach(function (input) {
      input.value = input.dataset.publicationInitialValue || "";
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
  });

  var MAX_SEGMENTS = 16;
  var MAX_TOKEN_SEGMENTS = 8;
  var MAX_TRIGRAM_SEGMENTS = 4;
  var MAX_DECODED_BYTES = 2 << 20;
  var MAX_POSTINGS = 10000;
  var MAX_RESULTS = 20;
  var MAX_TOKENS = 8;
  var MAX_RECENT = 6;

  function bindCatalogSidebarScrollPreservation() {
    if (window.__manjaCatalogSidebarScrollBound) return;
    window.__manjaCatalogSidebarScrollBound = true;
    var pendingSidebarScrollTop = null;

    function sidebarScrollPanel(root) {
      var scope = root && root.querySelector ? root : document;
      if (scope.matches && scope.matches(".sidebar-scroll")) return scope;
      var panel = scope.querySelector(".sidebar-scroll");
      if (panel) return panel;
      var groups = document.getElementById("catalog-sidebar-groups");
      return groups && groups.querySelector(".sidebar-scroll");
    }

    function sidebarSwapTarget(event) {
      var target = event.detail && (event.detail.ctx || event.detail.task)?.target;
      if (!target) return null;
      if (target.id === "catalog-sidebar-groups") {
        return document.getElementById("catalog-sidebar-groups") || target;
      }
      var groups = target.closest && target.closest("#catalog-sidebar-groups");
      return groups && (document.getElementById("catalog-sidebar-groups") || groups);
    }

    function restoreSidebarScroll(root, scrollTop) {
      var panel = sidebarScrollPanel(root);
      if (panel) panel.scrollTop = scrollTop;
    }

    document.body.addEventListener("htmx:before:request", function (event) {
      var trigger = event.detail && event.detail.ctx?.sourceElement;
      var control = trigger && trigger.closest && trigger.closest("[data-catalog-group-control]");
      if (!control) return;
      var panel = sidebarScrollPanel(control.closest("#catalog-sidebar-groups"));
      pendingSidebarScrollTop = panel ? panel.scrollTop : null;
    });
    document.body.addEventListener("htmx:after:swap", function (event) {
      var target = sidebarSwapTarget(event);
      if (target && pendingSidebarScrollTop !== null) {
        restoreSidebarScroll(target, pendingSidebarScrollTop);
      }
    });
    document.body.addEventListener("htmx:after:settle", function (event) {
      var target = sidebarSwapTarget(event);
      if (!target || pendingSidebarScrollTop === null) return;
      var scrollTop = pendingSidebarScrollTop;
      pendingSidebarScrollTop = null;
      restoreSidebarScroll(target, scrollTop);
      window.requestAnimationFrame(function () {
        window.requestAnimationFrame(function () {
          window.requestAnimationFrame(function () {
            restoreSidebarScroll(document, scrollTop);
          });
        });
      });
    });
    ["htmx:response:error", "htmx:error"].forEach(function (name) {
      document.body.addEventListener(name, function () { pendingSidebarScrollTop = null; });
    });
  }

  if (document.body) {
    bindCatalogSidebarScrollPreservation();
  } else {
    document.addEventListener("DOMContentLoaded", bindCatalogSidebarScrollPreservation, { once: true });
  }

  function usesCommandShortcut() {
    var platform = "";
    if (navigator.userAgentData && navigator.userAgentData.platform) {
      platform = navigator.userAgentData.platform;
    } else {
      platform = navigator.platform || navigator.userAgent || "";
    }
    return /Mac|iPhone|iPad|iPod/i.test(platform);
  }

  function syncPlatformShortcuts(root) {
    var scope = root && root.querySelectorAll ? root : document;
    var command = usesCommandShortcut();
    scope.querySelectorAll("[data-catalog-platform-shortcut]").forEach(function (wrapper) {
      var shortcut = wrapper.querySelector("kbd");
      if (!shortcut) return;
      shortcut.textContent = command ? "⌘ K" : "Ctrl K";
      shortcut.setAttribute("aria-label", command ? "Command K" : "Control K");
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", function () { syncPlatformShortcuts(document); }, { once: true });
  } else {
    syncPlatformShortcuts(document);
  }
  document.addEventListener("htmx:after:settle", function (event) {
    syncPlatformShortcuts(event.target || document);
  });

  function asString(value) {
    return value === null || value === undefined ? "" : String(value);
  }

  function utf8Length(value) {
    return new TextEncoder().encode(value).length;
  }

  function searchKindPriority(kind) {
    switch (asString(kind).toLowerCase()) {
      case "operation": return 0;
      case "schema": return 1;
      default: return 2;
    }
  }

  // BEGIN GENERATED SEARCH CASE FOLD
  // Code generated by cmd/webassets from golang.org/x/text/cases; DO NOT EDIT.
  var searchCaseFold = {"A":"a","B":"b","C":"c","D":"d","E":"e","F":"f","G":"g","H":"h","I":"i","J":"j","K":"k","L":"l","M":"m","N":"n","O":"o","P":"p","Q":"q","R":"r","S":"s","T":"t","U":"u","V":"v","W":"w","X":"x","Y":"y","Z":"z","µ":"μ","À":"à","Á":"á","Â":"â","Ã":"ã","Ä":"ä","Å":"å","Æ":"æ","Ç":"ç","È":"è","É":"é","Ê":"ê","Ë":"ë","Ì":"ì","Í":"í","Î":"î","Ï":"ï","Ð":"ð","Ñ":"ñ","Ò":"ò","Ó":"ó","Ô":"ô","Õ":"õ","Ö":"ö","Ø":"ø","Ù":"ù","Ú":"ú","Û":"û","Ü":"ü","Ý":"ý","Þ":"þ","ß":"ss","Ā":"ā","Ă":"ă","Ą":"ą","Ć":"ć","Ĉ":"ĉ","Ċ":"ċ","Č":"č","Ď":"ď","Đ":"đ","Ē":"ē","Ĕ":"ĕ","Ė":"ė","Ę":"ę","Ě":"ě","Ĝ":"ĝ","Ğ":"ğ","Ġ":"ġ","Ģ":"ģ","Ĥ":"ĥ","Ħ":"ħ","Ĩ":"ĩ","Ī":"ī","Ĭ":"ĭ","Į":"į","İ":"i̇","Ĳ":"ĳ","Ĵ":"ĵ","Ķ":"ķ","Ĺ":"ĺ","Ļ":"ļ","Ľ":"ľ","Ŀ":"ŀ","Ł":"ł","Ń":"ń","Ņ":"ņ","Ň":"ň","ŉ":"ʼn","Ŋ":"ŋ","Ō":"ō","Ŏ":"ŏ","Ő":"ő","Œ":"œ","Ŕ":"ŕ","Ŗ":"ŗ","Ř":"ř","Ś":"ś","Ŝ":"ŝ","Ş":"ş","Š":"š","Ţ":"ţ","Ť":"ť","Ŧ":"ŧ","Ũ":"ũ","Ū":"ū","Ŭ":"ŭ","Ů":"ů","Ű":"ű","Ų":"ų","Ŵ":"ŵ","Ŷ":"ŷ","Ÿ":"ÿ","Ź":"ź","Ż":"ż","Ž":"ž","ſ":"s","Ɓ":"ɓ","Ƃ":"ƃ","Ƅ":"ƅ","Ɔ":"ɔ","Ƈ":"ƈ","Ɖ":"ɖ","Ɗ":"ɗ","Ƌ":"ƌ","Ǝ":"ǝ","Ə":"ə","Ɛ":"ɛ","Ƒ":"ƒ","Ɠ":"ɠ","Ɣ":"ɣ","Ɩ":"ɩ","Ɨ":"ɨ","Ƙ":"ƙ","Ɯ":"ɯ","Ɲ":"ɲ","Ɵ":"ɵ","Ơ":"ơ","Ƣ":"ƣ","Ƥ":"ƥ","Ʀ":"ʀ","Ƨ":"ƨ","Ʃ":"ʃ","Ƭ":"ƭ","Ʈ":"ʈ","Ư":"ư","Ʊ":"ʊ","Ʋ":"ʋ","Ƴ":"ƴ","Ƶ":"ƶ","Ʒ":"ʒ","Ƹ":"ƹ","Ƽ":"ƽ","Ǆ":"ǆ","ǅ":"ǆ","Ǉ":"ǉ","ǈ":"ǉ","Ǌ":"ǌ","ǋ":"ǌ","Ǎ":"ǎ","Ǐ":"ǐ","Ǒ":"ǒ","Ǔ":"ǔ","Ǖ":"ǖ","Ǘ":"ǘ","Ǚ":"ǚ","Ǜ":"ǜ","Ǟ":"ǟ","Ǡ":"ǡ","Ǣ":"ǣ","Ǥ":"ǥ","Ǧ":"ǧ","Ǩ":"ǩ","Ǫ":"ǫ","Ǭ":"ǭ","Ǯ":"ǯ","ǰ":"ǰ","Ǳ":"ǳ","ǲ":"ǳ","Ǵ":"ǵ","Ƕ":"ƕ","Ƿ":"ƿ","Ǹ":"ǹ","Ǻ":"ǻ","Ǽ":"ǽ","Ǿ":"ǿ","Ȁ":"ȁ","Ȃ":"ȃ","Ȅ":"ȅ","Ȇ":"ȇ","Ȉ":"ȉ","Ȋ":"ȋ","Ȍ":"ȍ","Ȏ":"ȏ","Ȑ":"ȑ","Ȓ":"ȓ","Ȕ":"ȕ","Ȗ":"ȗ","Ș":"ș","Ț":"ț","Ȝ":"ȝ","Ȟ":"ȟ","Ƞ":"ƞ","Ȣ":"ȣ","Ȥ":"ȥ","Ȧ":"ȧ","Ȩ":"ȩ","Ȫ":"ȫ","Ȭ":"ȭ","Ȯ":"ȯ","Ȱ":"ȱ","Ȳ":"ȳ","Ⱥ":"ⱥ","Ȼ":"ȼ","Ƚ":"ƚ","Ⱦ":"ⱦ","Ɂ":"ɂ","Ƀ":"ƀ","Ʉ":"ʉ","Ʌ":"ʌ","Ɇ":"ɇ","Ɉ":"ɉ","Ɋ":"ɋ","Ɍ":"ɍ","Ɏ":"ɏ","ͅ":"ι","Ͱ":"ͱ","Ͳ":"ͳ","Ͷ":"ͷ","Ϳ":"ϳ","Ά":"ά","Έ":"έ","Ή":"ή","Ί":"ί","Ό":"ό","Ύ":"ύ","Ώ":"ώ","ΐ":"ΐ","Α":"α","Β":"β","Γ":"γ","Δ":"δ","Ε":"ε","Ζ":"ζ","Η":"η","Θ":"θ","Ι":"ι","Κ":"κ","Λ":"λ","Μ":"μ","Ν":"ν","Ξ":"ξ","Ο":"ο","Π":"π","Ρ":"ρ","Σ":"σ","Τ":"τ","Υ":"υ","Φ":"φ","Χ":"χ","Ψ":"ψ","Ω":"ω","Ϊ":"ϊ","Ϋ":"ϋ","ΰ":"ΰ","ς":"σ","Ϗ":"ϗ","ϐ":"β","ϑ":"θ","ϕ":"φ","ϖ":"π","Ϙ":"ϙ","Ϛ":"ϛ","Ϝ":"ϝ","Ϟ":"ϟ","Ϡ":"ϡ","Ϣ":"ϣ","Ϥ":"ϥ","Ϧ":"ϧ","Ϩ":"ϩ","Ϫ":"ϫ","Ϭ":"ϭ","Ϯ":"ϯ","ϰ":"κ","ϱ":"ρ","ϴ":"θ","ϵ":"ε","Ϸ":"ϸ","Ϲ":"ϲ","Ϻ":"ϻ","Ͻ":"ͻ","Ͼ":"ͼ","Ͽ":"ͽ","Ѐ":"ѐ","Ё":"ё","Ђ":"ђ","Ѓ":"ѓ","Є":"є","Ѕ":"ѕ","І":"і","Ї":"ї","Ј":"ј","Љ":"љ","Њ":"њ","Ћ":"ћ","Ќ":"ќ","Ѝ":"ѝ","Ў":"ў","Џ":"џ","А":"а","Б":"б","В":"в","Г":"г","Д":"д","Е":"е","Ж":"ж","З":"з","И":"и","Й":"й","К":"к","Л":"л","М":"м","Н":"н","О":"о","П":"п","Р":"р","С":"с","Т":"т","У":"у","Ф":"ф","Х":"х","Ц":"ц","Ч":"ч","Ш":"ш","Щ":"щ","Ъ":"ъ","Ы":"ы","Ь":"ь","Э":"э","Ю":"ю","Я":"я","Ѡ":"ѡ","Ѣ":"ѣ","Ѥ":"ѥ","Ѧ":"ѧ","Ѩ":"ѩ","Ѫ":"ѫ","Ѭ":"ѭ","Ѯ":"ѯ","Ѱ":"ѱ","Ѳ":"ѳ","Ѵ":"ѵ","Ѷ":"ѷ","Ѹ":"ѹ","Ѻ":"ѻ","Ѽ":"ѽ","Ѿ":"ѿ","Ҁ":"ҁ","Ҋ":"ҋ","Ҍ":"ҍ","Ҏ":"ҏ","Ґ":"ґ","Ғ":"ғ","Ҕ":"ҕ","Җ":"җ","Ҙ":"ҙ","Қ":"қ","Ҝ":"ҝ","Ҟ":"ҟ","Ҡ":"ҡ","Ң":"ң","Ҥ":"ҥ","Ҧ":"ҧ","Ҩ":"ҩ","Ҫ":"ҫ","Ҭ":"ҭ","Ү":"ү","Ұ":"ұ","Ҳ":"ҳ","Ҵ":"ҵ","Ҷ":"ҷ","Ҹ":"ҹ","Һ":"һ","Ҽ":"ҽ","Ҿ":"ҿ","Ӏ":"ӏ","Ӂ":"ӂ","Ӄ":"ӄ","Ӆ":"ӆ","Ӈ":"ӈ","Ӊ":"ӊ","Ӌ":"ӌ","Ӎ":"ӎ","Ӑ":"ӑ","Ӓ":"ӓ","Ӕ":"ӕ","Ӗ":"ӗ","Ә":"ә","Ӛ":"ӛ","Ӝ":"ӝ","Ӟ":"ӟ","Ӡ":"ӡ","Ӣ":"ӣ","Ӥ":"ӥ","Ӧ":"ӧ","Ө":"ө","Ӫ":"ӫ","Ӭ":"ӭ","Ӯ":"ӯ","Ӱ":"ӱ","Ӳ":"ӳ","Ӵ":"ӵ","Ӷ":"ӷ","Ӹ":"ӹ","Ӻ":"ӻ","Ӽ":"ӽ","Ӿ":"ӿ","Ԁ":"ԁ","Ԃ":"ԃ","Ԅ":"ԅ","Ԇ":"ԇ","Ԉ":"ԉ","Ԋ":"ԋ","Ԍ":"ԍ","Ԏ":"ԏ","Ԑ":"ԑ","Ԓ":"ԓ","Ԕ":"ԕ","Ԗ":"ԗ","Ԙ":"ԙ","Ԛ":"ԛ","Ԝ":"ԝ","Ԟ":"ԟ","Ԡ":"ԡ","Ԣ":"ԣ","Ԥ":"ԥ","Ԧ":"ԧ","Ԩ":"ԩ","Ԫ":"ԫ","Ԭ":"ԭ","Ԯ":"ԯ","Ա":"ա","Բ":"բ","Գ":"գ","Դ":"դ","Ե":"ե","Զ":"զ","Է":"է","Ը":"ը","Թ":"թ","Ժ":"ժ","Ի":"ի","Լ":"լ","Խ":"խ","Ծ":"ծ","Կ":"կ","Հ":"հ","Ձ":"ձ","Ղ":"ղ","Ճ":"ճ","Մ":"մ","Յ":"յ","Ն":"ն","Շ":"շ","Ո":"ո","Չ":"չ","Պ":"պ","Ջ":"ջ","Ռ":"ռ","Ս":"ս","Վ":"վ","Տ":"տ","Ր":"ր","Ց":"ց","Ւ":"ւ","Փ":"փ","Ք":"ք","Օ":"օ","Ֆ":"ֆ","և":"եւ","Ⴀ":"ⴀ","Ⴁ":"ⴁ","Ⴂ":"ⴂ","Ⴃ":"ⴃ","Ⴄ":"ⴄ","Ⴅ":"ⴅ","Ⴆ":"ⴆ","Ⴇ":"ⴇ","Ⴈ":"ⴈ","Ⴉ":"ⴉ","Ⴊ":"ⴊ","Ⴋ":"ⴋ","Ⴌ":"ⴌ","Ⴍ":"ⴍ","Ⴎ":"ⴎ","Ⴏ":"ⴏ","Ⴐ":"ⴐ","Ⴑ":"ⴑ","Ⴒ":"ⴒ","Ⴓ":"ⴓ","Ⴔ":"ⴔ","Ⴕ":"ⴕ","Ⴖ":"ⴖ","Ⴗ":"ⴗ","Ⴘ":"ⴘ","Ⴙ":"ⴙ","Ⴚ":"ⴚ","Ⴛ":"ⴛ","Ⴜ":"ⴜ","Ⴝ":"ⴝ","Ⴞ":"ⴞ","Ⴟ":"ⴟ","Ⴠ":"ⴠ","Ⴡ":"ⴡ","Ⴢ":"ⴢ","Ⴣ":"ⴣ","Ⴤ":"ⴤ","Ⴥ":"ⴥ","Ⴧ":"ⴧ","Ⴭ":"ⴭ","Ꭰ":"ꭰ","Ꭱ":"ꭱ","Ꭲ":"ꭲ","Ꭳ":"ꭳ","Ꭴ":"ꭴ","Ꭵ":"ꭵ","Ꭶ":"ꭶ","Ꭷ":"ꭷ","Ꭸ":"ꭸ","Ꭹ":"ꭹ","Ꭺ":"ꭺ","Ꭻ":"ꭻ","Ꭼ":"ꭼ","Ꭽ":"ꭽ","Ꭾ":"ꭾ","Ꭿ":"ꭿ","Ꮀ":"ꮀ","Ꮁ":"ꮁ","Ꮂ":"ꮂ","Ꮃ":"ꮃ","Ꮄ":"ꮄ","Ꮅ":"ꮅ","Ꮆ":"ꮆ","Ꮇ":"ꮇ","Ꮈ":"ꮈ","Ꮉ":"ꮉ","Ꮊ":"ꮊ","Ꮋ":"ꮋ","Ꮌ":"ꮌ","Ꮍ":"ꮍ","Ꮎ":"ꮎ","Ꮏ":"ꮏ","Ꮐ":"ꮐ","Ꮑ":"ꮑ","Ꮒ":"ꮒ","Ꮓ":"ꮓ","Ꮔ":"ꮔ","Ꮕ":"ꮕ","Ꮖ":"ꮖ","Ꮗ":"ꮗ","Ꮘ":"ꮘ","Ꮙ":"ꮙ","Ꮚ":"ꮚ","Ꮛ":"ꮛ","Ꮜ":"ꮜ","Ꮝ":"ꮝ","Ꮞ":"ꮞ","Ꮟ":"ꮟ","Ꮠ":"ꮠ","Ꮡ":"ꮡ","Ꮢ":"ꮢ","Ꮣ":"ꮣ","Ꮤ":"ꮤ","Ꮥ":"ꮥ","Ꮦ":"ꮦ","Ꮧ":"ꮧ","Ꮨ":"ꮨ","Ꮩ":"ꮩ","Ꮪ":"ꮪ","Ꮫ":"ꮫ","Ꮬ":"ꮬ","Ꮭ":"ꮭ","Ꮮ":"ꮮ","Ꮯ":"ꮯ","Ꮰ":"ꮰ","Ꮱ":"ꮱ","Ꮲ":"ꮲ","Ꮳ":"ꮳ","Ꮴ":"ꮴ","Ꮵ":"ꮵ","Ꮶ":"ꮶ","Ꮷ":"ꮷ","Ꮸ":"ꮸ","Ꮹ":"ꮹ","Ꮺ":"ꮺ","Ꮻ":"ꮻ","Ꮼ":"ꮼ","Ꮽ":"ꮽ","Ꮾ":"ꮾ","Ꮿ":"ꮿ","Ᏸ":"ᏸ","Ᏹ":"ᏹ","Ᏺ":"ᏺ","Ᏻ":"ᏻ","Ᏼ":"ᏼ","Ᏽ":"ᏽ","ᏸ":"Ᏸ","ᏹ":"Ᏹ","ᏺ":"Ᏺ","ᏻ":"Ᏻ","ᏼ":"Ᏼ","ᏽ":"Ᏽ","ᲀ":"в","ᲁ":"д","ᲂ":"о","ᲃ":"с","ᲄ":"т","ᲅ":"т","ᲆ":"ъ","ᲇ":"ѣ","ᲈ":"ꙋ","Ᲊ":"ᲊ","Ა":"ა","Ბ":"ბ","Გ":"გ","Დ":"დ","Ე":"ე","Ვ":"ვ","Ზ":"ზ","Თ":"თ","Ი":"ი","Კ":"კ","Ლ":"ლ","Მ":"მ","Ნ":"ნ","Ო":"ო","Პ":"პ","Ჟ":"ჟ","Რ":"რ","Ს":"ს","Ტ":"ტ","Უ":"უ","Ფ":"ფ","Ქ":"ქ","Ღ":"ღ","Ყ":"ყ","Შ":"შ","Ჩ":"ჩ","Ც":"ც","Ძ":"ძ","Წ":"წ","Ჭ":"ჭ","Ხ":"ხ","Ჯ":"ჯ","Ჰ":"ჰ","Ჱ":"ჱ","Ჲ":"ჲ","Ჳ":"ჳ","Ჴ":"ჴ","Ჵ":"ჵ","Ჶ":"ჶ","Ჷ":"ჷ","Ჸ":"ჸ","Ჹ":"ჹ","Ჺ":"ჺ","Ჽ":"ჽ","Ჾ":"ჾ","Ჿ":"ჿ","Ḁ":"ḁ","Ḃ":"ḃ","Ḅ":"ḅ","Ḇ":"ḇ","Ḉ":"ḉ","Ḋ":"ḋ","Ḍ":"ḍ","Ḏ":"ḏ","Ḑ":"ḑ","Ḓ":"ḓ","Ḕ":"ḕ","Ḗ":"ḗ","Ḙ":"ḙ","Ḛ":"ḛ","Ḝ":"ḝ","Ḟ":"ḟ","Ḡ":"ḡ","Ḣ":"ḣ","Ḥ":"ḥ","Ḧ":"ḧ","Ḩ":"ḩ","Ḫ":"ḫ","Ḭ":"ḭ","Ḯ":"ḯ","Ḱ":"ḱ","Ḳ":"ḳ","Ḵ":"ḵ","Ḷ":"ḷ","Ḹ":"ḹ","Ḻ":"ḻ","Ḽ":"ḽ","Ḿ":"ḿ","Ṁ":"ṁ","Ṃ":"ṃ","Ṅ":"ṅ","Ṇ":"ṇ","Ṉ":"ṉ","Ṋ":"ṋ","Ṍ":"ṍ","Ṏ":"ṏ","Ṑ":"ṑ","Ṓ":"ṓ","Ṕ":"ṕ","Ṗ":"ṗ","Ṙ":"ṙ","Ṛ":"ṛ","Ṝ":"ṝ","Ṟ":"ṟ","Ṡ":"ṡ","Ṣ":"ṣ","Ṥ":"ṥ","Ṧ":"ṧ","Ṩ":"ṩ","Ṫ":"ṫ","Ṭ":"ṭ","Ṯ":"ṯ","Ṱ":"ṱ","Ṳ":"ṳ","Ṵ":"ṵ","Ṷ":"ṷ","Ṹ":"ṹ","Ṻ":"ṻ","Ṽ":"ṽ","Ṿ":"ṿ","Ẁ":"ẁ","Ẃ":"ẃ","Ẅ":"ẅ","Ẇ":"ẇ","Ẉ":"ẉ","Ẋ":"ẋ","Ẍ":"ẍ","Ẏ":"ẏ","Ẑ":"ẑ","Ẓ":"ẓ","Ẕ":"ẕ","ẖ":"ẖ","ẗ":"ẗ","ẘ":"ẘ","ẙ":"ẙ","ẚ":"aʾ","ẛ":"ṡ","ẞ":"ss","Ạ":"ạ","Ả":"ả","Ấ":"ấ","Ầ":"ầ","Ẩ":"ẩ","Ẫ":"ẫ","Ậ":"ậ","Ắ":"ắ","Ằ":"ằ","Ẳ":"ẳ","Ẵ":"ẵ","Ặ":"ặ","Ẹ":"ẹ","Ẻ":"ẻ","Ẽ":"ẽ","Ế":"ế","Ề":"ề","Ể":"ể","Ễ":"ễ","Ệ":"ệ","Ỉ":"ỉ","Ị":"ị","Ọ":"ọ","Ỏ":"ỏ","Ố":"ố","Ồ":"ồ","Ổ":"ổ","Ỗ":"ỗ","Ộ":"ộ","Ớ":"ớ","Ờ":"ờ","Ở":"ở","Ỡ":"ỡ","Ợ":"ợ","Ụ":"ụ","Ủ":"ủ","Ứ":"ứ","Ừ":"ừ","Ử":"ử","Ữ":"ữ","Ự":"ự","Ỳ":"ỳ","Ỵ":"ỵ","Ỷ":"ỷ","Ỹ":"ỹ","Ỻ":"ỻ","Ỽ":"ỽ","Ỿ":"ỿ","Ἀ":"ἀ","Ἁ":"ἁ","Ἂ":"ἂ","Ἃ":"ἃ","Ἄ":"ἄ","Ἅ":"ἅ","Ἆ":"ἆ","Ἇ":"ἇ","Ἐ":"ἐ","Ἑ":"ἑ","Ἒ":"ἒ","Ἓ":"ἓ","Ἔ":"ἔ","Ἕ":"ἕ","Ἠ":"ἠ","Ἡ":"ἡ","Ἢ":"ἢ","Ἣ":"ἣ","Ἤ":"ἤ","Ἥ":"ἥ","Ἦ":"ἦ","Ἧ":"ἧ","Ἰ":"ἰ","Ἱ":"ἱ","Ἲ":"ἲ","Ἳ":"ἳ","Ἴ":"ἴ","Ἵ":"ἵ","Ἶ":"ἶ","Ἷ":"ἷ","Ὀ":"ὀ","Ὁ":"ὁ","Ὂ":"ὂ","Ὃ":"ὃ","Ὄ":"ὄ","Ὅ":"ὅ","ὐ":"ὐ","ὒ":"ὒ","ὔ":"ὔ","ὖ":"ὖ","Ὑ":"ὑ","Ὓ":"ὓ","Ὕ":"ὕ","Ὗ":"ὗ","Ὠ":"ὠ","Ὡ":"ὡ","Ὢ":"ὢ","Ὣ":"ὣ","Ὤ":"ὤ","Ὥ":"ὥ","Ὦ":"ὦ","Ὧ":"ὧ","ᾀ":"ἀι","ᾁ":"ἁι","ᾂ":"ἂι","ᾃ":"ἃι","ᾄ":"ἄι","ᾅ":"ἅι","ᾆ":"ἆι","ᾇ":"ἇι","ᾈ":"ἀι","ᾉ":"ἁι","ᾊ":"ἂι","ᾋ":"ἃι","ᾌ":"ἄι","ᾍ":"ἅι","ᾎ":"ἆι","ᾏ":"ἇι","ᾐ":"ἠι","ᾑ":"ἡι","ᾒ":"ἢι","ᾓ":"ἣι","ᾔ":"ἤι","ᾕ":"ἥι","ᾖ":"ἦι","ᾗ":"ἧι","ᾘ":"ἠι","ᾙ":"ἡι","ᾚ":"ἢι","ᾛ":"ἣι","ᾜ":"ἤι","ᾝ":"ἥι","ᾞ":"ἦι","ᾟ":"ἧι","ᾠ":"ὠι","ᾡ":"ὡι","ᾢ":"ὢι","ᾣ":"ὣι","ᾤ":"ὤι","ᾥ":"ὥι","ᾦ":"ὦι","ᾧ":"ὧι","ᾨ":"ὠι","ᾩ":"ὡι","ᾪ":"ὢι","ᾫ":"ὣι","ᾬ":"ὤι","ᾭ":"ὥι","ᾮ":"ὦι","ᾯ":"ὧι","ᾲ":"ὰι","ᾳ":"αι","ᾴ":"άι","ᾶ":"ᾶ","ᾷ":"ᾶι","Ᾰ":"ᾰ","Ᾱ":"ᾱ","Ὰ":"ὰ","Ά":"ά","ᾼ":"αι","ι":"ι","ῂ":"ὴι","ῃ":"ηι","ῄ":"ήι","ῆ":"ῆ","ῇ":"ῆι","Ὲ":"ὲ","Έ":"έ","Ὴ":"ὴ","Ή":"ή","ῌ":"ηι","ῒ":"ῒ","ΐ":"ΐ","ῖ":"ῖ","ῗ":"ῗ","Ῐ":"ῐ","Ῑ":"ῑ","Ὶ":"ὶ","Ί":"ί","ῢ":"ῢ","ΰ":"ΰ","ῤ":"ῤ","ῦ":"ῦ","ῧ":"ῧ","Ῠ":"ῠ","Ῡ":"ῡ","Ὺ":"ὺ","Ύ":"ύ","Ῥ":"ῥ","ῲ":"ὼι","ῳ":"ωι","ῴ":"ώι","ῶ":"ῶ","ῷ":"ῶι","Ὸ":"ὸ","Ό":"ό","Ὼ":"ὼ","Ώ":"ώ","ῼ":"ωι","Ω":"ω","K":"k","Å":"å","Ⅎ":"ⅎ","Ⅰ":"ⅰ","Ⅱ":"ⅱ","Ⅲ":"ⅲ","Ⅳ":"ⅳ","Ⅴ":"ⅴ","Ⅵ":"ⅵ","Ⅶ":"ⅶ","Ⅷ":"ⅷ","Ⅸ":"ⅸ","Ⅹ":"ⅹ","Ⅺ":"ⅺ","Ⅻ":"ⅻ","Ⅼ":"ⅼ","Ⅽ":"ⅽ","Ⅾ":"ⅾ","Ⅿ":"ⅿ","Ↄ":"ↄ","Ⓐ":"ⓐ","Ⓑ":"ⓑ","Ⓒ":"ⓒ","Ⓓ":"ⓓ","Ⓔ":"ⓔ","Ⓕ":"ⓕ","Ⓖ":"ⓖ","Ⓗ":"ⓗ","Ⓘ":"ⓘ","Ⓙ":"ⓙ","Ⓚ":"ⓚ","Ⓛ":"ⓛ","Ⓜ":"ⓜ","Ⓝ":"ⓝ","Ⓞ":"ⓞ","Ⓟ":"ⓟ","Ⓠ":"ⓠ","Ⓡ":"ⓡ","Ⓢ":"ⓢ","Ⓣ":"ⓣ","Ⓤ":"ⓤ","Ⓥ":"ⓥ","Ⓦ":"ⓦ","Ⓧ":"ⓧ","Ⓨ":"ⓨ","Ⓩ":"ⓩ","Ⰰ":"ⰰ","Ⰱ":"ⰱ","Ⰲ":"ⰲ","Ⰳ":"ⰳ","Ⰴ":"ⰴ","Ⰵ":"ⰵ","Ⰶ":"ⰶ","Ⰷ":"ⰷ","Ⰸ":"ⰸ","Ⰹ":"ⰹ","Ⰺ":"ⰺ","Ⰻ":"ⰻ","Ⰼ":"ⰼ","Ⰽ":"ⰽ","Ⰾ":"ⰾ","Ⰿ":"ⰿ","Ⱀ":"ⱀ","Ⱁ":"ⱁ","Ⱂ":"ⱂ","Ⱃ":"ⱃ","Ⱄ":"ⱄ","Ⱅ":"ⱅ","Ⱆ":"ⱆ","Ⱇ":"ⱇ","Ⱈ":"ⱈ","Ⱉ":"ⱉ","Ⱊ":"ⱊ","Ⱋ":"ⱋ","Ⱌ":"ⱌ","Ⱍ":"ⱍ","Ⱎ":"ⱎ","Ⱏ":"ⱏ","Ⱐ":"ⱐ","Ⱑ":"ⱑ","Ⱒ":"ⱒ","Ⱓ":"ⱓ","Ⱔ":"ⱔ","Ⱕ":"ⱕ","Ⱖ":"ⱖ","Ⱗ":"ⱗ","Ⱘ":"ⱘ","Ⱙ":"ⱙ","Ⱚ":"ⱚ","Ⱛ":"ⱛ","Ⱜ":"ⱜ","Ⱝ":"ⱝ","Ⱞ":"ⱞ","Ⱟ":"ⱟ","Ⱡ":"ⱡ","Ɫ":"ɫ","Ᵽ":"ᵽ","Ɽ":"ɽ","Ⱨ":"ⱨ","Ⱪ":"ⱪ","Ⱬ":"ⱬ","Ɑ":"ɑ","Ɱ":"ɱ","Ɐ":"ɐ","Ɒ":"ɒ","Ⱳ":"ⱳ","Ⱶ":"ⱶ","Ȿ":"ȿ","Ɀ":"ɀ","Ⲁ":"ⲁ","Ⲃ":"ⲃ","Ⲅ":"ⲅ","Ⲇ":"ⲇ","Ⲉ":"ⲉ","Ⲋ":"ⲋ","Ⲍ":"ⲍ","Ⲏ":"ⲏ","Ⲑ":"ⲑ","Ⲓ":"ⲓ","Ⲕ":"ⲕ","Ⲗ":"ⲗ","Ⲙ":"ⲙ","Ⲛ":"ⲛ","Ⲝ":"ⲝ","Ⲟ":"ⲟ","Ⲡ":"ⲡ","Ⲣ":"ⲣ","Ⲥ":"ⲥ","Ⲧ":"ⲧ","Ⲩ":"ⲩ","Ⲫ":"ⲫ","Ⲭ":"ⲭ","Ⲯ":"ⲯ","Ⲱ":"ⲱ","Ⲳ":"ⲳ","Ⲵ":"ⲵ","Ⲷ":"ⲷ","Ⲹ":"ⲹ","Ⲻ":"ⲻ","Ⲽ":"ⲽ","Ⲿ":"ⲿ","Ⳁ":"ⳁ","Ⳃ":"ⳃ","Ⳅ":"ⳅ","Ⳇ":"ⳇ","Ⳉ":"ⳉ","Ⳋ":"ⳋ","Ⳍ":"ⳍ","Ⳏ":"ⳏ","Ⳑ":"ⳑ","Ⳓ":"ⳓ","Ⳕ":"ⳕ","Ⳗ":"ⳗ","Ⳙ":"ⳙ","Ⳛ":"ⳛ","Ⳝ":"ⳝ","Ⳟ":"ⳟ","Ⳡ":"ⳡ","Ⳣ":"ⳣ","Ⳬ":"ⳬ","Ⳮ":"ⳮ","Ⳳ":"ⳳ","Ꙁ":"ꙁ","Ꙃ":"ꙃ","Ꙅ":"ꙅ","Ꙇ":"ꙇ","Ꙉ":"ꙉ","Ꙋ":"ꙋ","Ꙍ":"ꙍ","Ꙏ":"ꙏ","Ꙑ":"ꙑ","Ꙓ":"ꙓ","Ꙕ":"ꙕ","Ꙗ":"ꙗ","Ꙙ":"ꙙ","Ꙛ":"ꙛ","Ꙝ":"ꙝ","Ꙟ":"ꙟ","Ꙡ":"ꙡ","Ꙣ":"ꙣ","Ꙥ":"ꙥ","Ꙧ":"ꙧ","Ꙩ":"ꙩ","Ꙫ":"ꙫ","Ꙭ":"ꙭ","Ꚁ":"ꚁ","Ꚃ":"ꚃ","Ꚅ":"ꚅ","Ꚇ":"ꚇ","Ꚉ":"ꚉ","Ꚋ":"ꚋ","Ꚍ":"ꚍ","Ꚏ":"ꚏ","Ꚑ":"ꚑ","Ꚓ":"ꚓ","Ꚕ":"ꚕ","Ꚗ":"ꚗ","Ꚙ":"ꚙ","Ꚛ":"ꚛ","Ꜣ":"ꜣ","Ꜥ":"ꜥ","Ꜧ":"ꜧ","Ꜩ":"ꜩ","Ꜫ":"ꜫ","Ꜭ":"ꜭ","Ꜯ":"ꜯ","Ꜳ":"ꜳ","Ꜵ":"ꜵ","Ꜷ":"ꜷ","Ꜹ":"ꜹ","Ꜻ":"ꜻ","Ꜽ":"ꜽ","Ꜿ":"ꜿ","Ꝁ":"ꝁ","Ꝃ":"ꝃ","Ꝅ":"ꝅ","Ꝇ":"ꝇ","Ꝉ":"ꝉ","Ꝋ":"ꝋ","Ꝍ":"ꝍ","Ꝏ":"ꝏ","Ꝑ":"ꝑ","Ꝓ":"ꝓ","Ꝕ":"ꝕ","Ꝗ":"ꝗ","Ꝙ":"ꝙ","Ꝛ":"ꝛ","Ꝝ":"ꝝ","Ꝟ":"ꝟ","Ꝡ":"ꝡ","Ꝣ":"ꝣ","Ꝥ":"ꝥ","Ꝧ":"ꝧ","Ꝩ":"ꝩ","Ꝫ":"ꝫ","Ꝭ":"ꝭ","Ꝯ":"ꝯ","Ꝺ":"ꝺ","Ꝼ":"ꝼ","Ᵹ":"ᵹ","Ꝿ":"ꝿ","Ꞁ":"ꞁ","Ꞃ":"ꞃ","Ꞅ":"ꞅ","Ꞇ":"ꞇ","Ꞌ":"ꞌ","Ɥ":"ɥ","Ꞑ":"ꞑ","Ꞓ":"ꞓ","Ꞗ":"ꞗ","Ꞙ":"ꞙ","Ꞛ":"ꞛ","Ꞝ":"ꞝ","Ꞟ":"ꞟ","Ꞡ":"ꞡ","Ꞣ":"ꞣ","Ꞥ":"ꞥ","Ꞧ":"ꞧ","Ꞩ":"ꞩ","Ɦ":"ɦ","Ɜ":"ɜ","Ɡ":"ɡ","Ɬ":"ɬ","Ɪ":"ɪ","Ʞ":"ʞ","Ʇ":"ʇ","Ʝ":"ʝ","Ꭓ":"ꭓ","Ꞵ":"ꞵ","Ꞷ":"ꞷ","Ꞹ":"ꞹ","Ꞻ":"ꞻ","Ꞽ":"ꞽ","Ꞿ":"ꞿ","Ꟁ":"ꟁ","Ꟃ":"ꟃ","Ꞔ":"ꞔ","Ʂ":"ʂ","Ᶎ":"ᶎ","Ꟈ":"ꟈ","Ꟊ":"ꟊ","Ɤ":"ɤ","Ꟍ":"ꟍ","꟎":"꟏","Ꟑ":"ꟑ","꟒":"ꟓ","꟔":"ꟕ","Ꟗ":"ꟗ","Ꟙ":"ꟙ","Ꟛ":"ꟛ","Ƛ":"ƛ","Ꟶ":"ꟶ","ꭰ":"Ꭰ","ꭱ":"Ꭱ","ꭲ":"Ꭲ","ꭳ":"Ꭳ","ꭴ":"Ꭴ","ꭵ":"Ꭵ","ꭶ":"Ꭶ","ꭷ":"Ꭷ","ꭸ":"Ꭸ","ꭹ":"Ꭹ","ꭺ":"Ꭺ","ꭻ":"Ꭻ","ꭼ":"Ꭼ","ꭽ":"Ꭽ","ꭾ":"Ꭾ","ꭿ":"Ꭿ","ꮀ":"Ꮀ","ꮁ":"Ꮁ","ꮂ":"Ꮂ","ꮃ":"Ꮃ","ꮄ":"Ꮄ","ꮅ":"Ꮅ","ꮆ":"Ꮆ","ꮇ":"Ꮇ","ꮈ":"Ꮈ","ꮉ":"Ꮉ","ꮊ":"Ꮊ","ꮋ":"Ꮋ","ꮌ":"Ꮌ","ꮍ":"Ꮍ","ꮎ":"Ꮎ","ꮏ":"Ꮏ","ꮐ":"Ꮐ","ꮑ":"Ꮑ","ꮒ":"Ꮒ","ꮓ":"Ꮓ","ꮔ":"Ꮔ","ꮕ":"Ꮕ","ꮖ":"Ꮖ","ꮗ":"Ꮗ","ꮘ":"Ꮘ","ꮙ":"Ꮙ","ꮚ":"Ꮚ","ꮛ":"Ꮛ","ꮜ":"Ꮜ","ꮝ":"Ꮝ","ꮞ":"Ꮞ","ꮟ":"Ꮟ","ꮠ":"Ꮠ","ꮡ":"Ꮡ","ꮢ":"Ꮢ","ꮣ":"Ꮣ","ꮤ":"Ꮤ","ꮥ":"Ꮥ","ꮦ":"Ꮦ","ꮧ":"Ꮧ","ꮨ":"Ꮨ","ꮩ":"Ꮩ","ꮪ":"Ꮪ","ꮫ":"Ꮫ","ꮬ":"Ꮬ","ꮭ":"Ꮭ","ꮮ":"Ꮮ","ꮯ":"Ꮯ","ꮰ":"Ꮰ","ꮱ":"Ꮱ","ꮲ":"Ꮲ","ꮳ":"Ꮳ","ꮴ":"Ꮴ","ꮵ":"Ꮵ","ꮶ":"Ꮶ","ꮷ":"Ꮷ","ꮸ":"Ꮸ","ꮹ":"Ꮹ","ꮺ":"Ꮺ","ꮻ":"Ꮻ","ꮼ":"Ꮼ","ꮽ":"Ꮽ","ꮾ":"Ꮾ","ꮿ":"Ꮿ","ﬀ":"ff","ﬁ":"fi","ﬂ":"fl","ﬃ":"ffi","ﬄ":"ffl","ﬅ":"st","ﬆ":"st","ﬓ":"մն","ﬔ":"մե","ﬕ":"մի","ﬖ":"վն","ﬗ":"մխ","Ａ":"ａ","Ｂ":"ｂ","Ｃ":"ｃ","Ｄ":"ｄ","Ｅ":"ｅ","Ｆ":"ｆ","Ｇ":"ｇ","Ｈ":"ｈ","Ｉ":"ｉ","Ｊ":"ｊ","Ｋ":"ｋ","Ｌ":"ｌ","Ｍ":"ｍ","Ｎ":"ｎ","Ｏ":"ｏ","Ｐ":"ｐ","Ｑ":"ｑ","Ｒ":"ｒ","Ｓ":"ｓ","Ｔ":"ｔ","Ｕ":"ｕ","Ｖ":"ｖ","Ｗ":"ｗ","Ｘ":"ｘ","Ｙ":"ｙ","Ｚ":"ｚ","𐐀":"𐐨","𐐁":"𐐩","𐐂":"𐐪","𐐃":"𐐫","𐐄":"𐐬","𐐅":"𐐭","𐐆":"𐐮","𐐇":"𐐯","𐐈":"𐐰","𐐉":"𐐱","𐐊":"𐐲","𐐋":"𐐳","𐐌":"𐐴","𐐍":"𐐵","𐐎":"𐐶","𐐏":"𐐷","𐐐":"𐐸","𐐑":"𐐹","𐐒":"𐐺","𐐓":"𐐻","𐐔":"𐐼","𐐕":"𐐽","𐐖":"𐐾","𐐗":"𐐿","𐐘":"𐑀","𐐙":"𐑁","𐐚":"𐑂","𐐛":"𐑃","𐐜":"𐑄","𐐝":"𐑅","𐐞":"𐑆","𐐟":"𐑇","𐐠":"𐑈","𐐡":"𐑉","𐐢":"𐑊","𐐣":"𐑋","𐐤":"𐑌","𐐥":"𐑍","𐐦":"𐑎","𐐧":"𐑏","𐒰":"𐓘","𐒱":"𐓙","𐒲":"𐓚","𐒳":"𐓛","𐒴":"𐓜","𐒵":"𐓝","𐒶":"𐓞","𐒷":"𐓟","𐒸":"𐓠","𐒹":"𐓡","𐒺":"𐓢","𐒻":"𐓣","𐒼":"𐓤","𐒽":"𐓥","𐒾":"𐓦","𐒿":"𐓧","𐓀":"𐓨","𐓁":"𐓩","𐓂":"𐓪","𐓃":"𐓫","𐓄":"𐓬","𐓅":"𐓭","𐓆":"𐓮","𐓇":"𐓯","𐓈":"𐓰","𐓉":"𐓱","𐓊":"𐓲","𐓋":"𐓳","𐓌":"𐓴","𐓍":"𐓵","𐓎":"𐓶","𐓏":"𐓷","𐓐":"𐓸","𐓑":"𐓹","𐓒":"𐓺","𐓓":"𐓻","𐕰":"𐖗","𐕱":"𐖘","𐕲":"𐖙","𐕳":"𐖚","𐕴":"𐖛","𐕵":"𐖜","𐕶":"𐖝","𐕷":"𐖞","𐕸":"𐖟","𐕹":"𐖠","𐕺":"𐖡","𐕼":"𐖣","𐕽":"𐖤","𐕾":"𐖥","𐕿":"𐖦","𐖀":"𐖧","𐖁":"𐖨","𐖂":"𐖩","𐖃":"𐖪","𐖄":"𐖫","𐖅":"𐖬","𐖆":"𐖭","𐖇":"𐖮","𐖈":"𐖯","𐖉":"𐖰","𐖊":"𐖱","𐖌":"𐖳","𐖍":"𐖴","𐖎":"𐖵","𐖏":"𐖶","𐖐":"𐖷","𐖑":"𐖸","𐖒":"𐖹","𐖔":"𐖻","𐖕":"𐖼","𐲀":"𐳀","𐲁":"𐳁","𐲂":"𐳂","𐲃":"𐳃","𐲄":"𐳄","𐲅":"𐳅","𐲆":"𐳆","𐲇":"𐳇","𐲈":"𐳈","𐲉":"𐳉","𐲊":"𐳊","𐲋":"𐳋","𐲌":"𐳌","𐲍":"𐳍","𐲎":"𐳎","𐲏":"𐳏","𐲐":"𐳐","𐲑":"𐳑","𐲒":"𐳒","𐲓":"𐳓","𐲔":"𐳔","𐲕":"𐳕","𐲖":"𐳖","𐲗":"𐳗","𐲘":"𐳘","𐲙":"𐳙","𐲚":"𐳚","𐲛":"𐳛","𐲜":"𐳜","𐲝":"𐳝","𐲞":"𐳞","𐲟":"𐳟","𐲠":"𐳠","𐲡":"𐳡","𐲢":"𐳢","𐲣":"𐳣","𐲤":"𐳤","𐲥":"𐳥","𐲦":"𐳦","𐲧":"𐳧","𐲨":"𐳨","𐲩":"𐳩","𐲪":"𐳪","𐲫":"𐳫","𐲬":"𐳬","𐲭":"𐳭","𐲮":"𐳮","𐲯":"𐳯","𐲰":"𐳰","𐲱":"𐳱","𐲲":"𐳲","𐵐":"𐵰","𐵑":"𐵱","𐵒":"𐵲","𐵓":"𐵳","𐵔":"𐵴","𐵕":"𐵵","𐵖":"𐵶","𐵗":"𐵷","𐵘":"𐵸","𐵙":"𐵹","𐵚":"𐵺","𐵛":"𐵻","𐵜":"𐵼","𐵝":"𐵽","𐵞":"𐵾","𐵟":"𐵿","𐵠":"𐶀","𐵡":"𐶁","𐵢":"𐶂","𐵣":"𐶃","𐵤":"𐶄","𐵥":"𐶅","𑢠":"𑣀","𑢡":"𑣁","𑢢":"𑣂","𑢣":"𑣃","𑢤":"𑣄","𑢥":"𑣅","𑢦":"𑣆","𑢧":"𑣇","𑢨":"𑣈","𑢩":"𑣉","𑢪":"𑣊","𑢫":"𑣋","𑢬":"𑣌","𑢭":"𑣍","𑢮":"𑣎","𑢯":"𑣏","𑢰":"𑣐","𑢱":"𑣑","𑢲":"𑣒","𑢳":"𑣓","𑢴":"𑣔","𑢵":"𑣕","𑢶":"𑣖","𑢷":"𑣗","𑢸":"𑣘","𑢹":"𑣙","𑢺":"𑣚","𑢻":"𑣛","𑢼":"𑣜","𑢽":"𑣝","𑢾":"𑣞","𑢿":"𑣟","𖹀":"𖹠","𖹁":"𖹡","𖹂":"𖹢","𖹃":"𖹣","𖹄":"𖹤","𖹅":"𖹥","𖹆":"𖹦","𖹇":"𖹧","𖹈":"𖹨","𖹉":"𖹩","𖹊":"𖹪","𖹋":"𖹫","𖹌":"𖹬","𖹍":"𖹭","𖹎":"𖹮","𖹏":"𖹯","𖹐":"𖹰","𖹑":"𖹱","𖹒":"𖹲","𖹓":"𖹳","𖹔":"𖹴","𖹕":"𖹵","𖹖":"𖹶","𖹗":"𖹷","𖹘":"𖹸","𖹙":"𖹹","𖹚":"𖹺","𖹛":"𖹻","𖹜":"𖹼","𖹝":"𖹽","𖹞":"𖹾","𖹟":"𖹿","𖺠":"𖺻","𖺡":"𖺼","𖺢":"𖺽","𖺣":"𖺾","𖺤":"𖺿","𖺥":"𖻀","𖺦":"𖻁","𖺧":"𖻂","𖺨":"𖻃","𖺩":"𖻄","𖺪":"𖻅","𖺫":"𖻆","𖺬":"𖻇","𖺭":"𖻈","𖺮":"𖻉","𖺯":"𖻊","𖺰":"𖻋","𖺱":"𖻌","𖺲":"𖻍","𖺳":"𖻎","𖺴":"𖻏","𖺵":"𖻐","𖺶":"𖻑","𖺷":"𖻒","𖺸":"𖻓","𞤀":"𞤢","𞤁":"𞤣","𞤂":"𞤤","𞤃":"𞤥","𞤄":"𞤦","𞤅":"𞤧","𞤆":"𞤨","𞤇":"𞤩","𞤈":"𞤪","𞤉":"𞤫","𞤊":"𞤬","𞤋":"𞤭","𞤌":"𞤮","𞤍":"𞤯","𞤎":"𞤰","𞤏":"𞤱","𞤐":"𞤲","𞤑":"𞤳","𞤒":"𞤴","𞤓":"𞤵","𞤔":"𞤶","𞤕":"𞤷","𞤖":"𞤸","𞤗":"𞤹","𞤘":"𞤺","𞤙":"𞤻","𞤚":"𞤼","𞤛":"𞤽","𞤜":"𞤾","𞤝":"𞤿","𞤞":"𞥀","𞤟":"𞥁","𞤠":"𞥂","𞤡":"𞥃"};
  // END GENERATED SEARCH CASE FOLD

  function foldSearchText(input) {
    return Array.from(input.normalize("NFKC"), function (character) {
      return searchCaseFold[character] || character;
    }).join("");
  }

  function trimSearchText(input) {
    return input.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, "");
  }

  // Indexed text is not a user query: titles may be empty, multiline, Unicode,
  // or longer than the query limits. Fold it for comparison without rejecting it.
  function normalizeCorpus(input) {
    return trimSearchText(foldSearchText(asString(input).replace(/[\u0000-\u001f\u007f-\u009f]+/g, " ")));
  }

  function normalizeExact(input) {
    var value = asString(input);
    if (utf8Length(value) > 256 || /[\p{Cc}\p{Cs}]/u.test(value)) {
      throw new Error("Invalid search query");
    }
    value = foldSearchText(trimSearchText(value));
    if (!value || utf8Length(value) > 256 || Array.from(value).length > 128) {
      throw new Error("Invalid search query");
    }
    return value;
  }

  function tokenize(value) {
    var tokens = [];
    var token = "";
    Array.from(value).forEach(function (character) {
      if (/[\p{L}\p{Nd}/{}.:_-]/u.test(character)) token += character;
      else if (token) {
        tokens.push(token);
        token = "";
      }
    });
    if (token) tokens.push(token);
    if (!tokens.length || tokens.length > MAX_TOKENS) throw new Error("Invalid search query");
    return tokens;
  }

  function trigrams(value) {
    var runes = Array.from(value);
    if (runes.length < 3) return [];
    var count = runes.length - 2;
    var positions = [0];
    if (count > 1) positions.push(Math.floor(count / 2));
    if (count > 2) positions.push(count - 1);
    var values = [];
    positions.forEach(function (position) {
      var trigram = runes.slice(position, position + 3).join("");
      if (trigram.length && values.indexOf(trigram) < 0) values.push(trigram);
    });
    return values.sort();
  }

  function escapeHTML(value) {
    return asString(value).replace(/[&<>"']/g, function (character) {
      return {
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#39;",
      }[character];
    });
  }

  function markNeedle(characters, needle, marks) {
    var matched = false;
    if (!needle.length || needle.length > characters.length) return matched;
    for (var start = 0; start <= characters.length - needle.length; start++) {
      var equal = true;
      for (var offset = 0; offset < needle.length; offset++) {
        if (characters[start + offset] !== needle[offset]) {
          equal = false;
          break;
        }
      }
      if (!equal) continue;
      matched = true;
      for (var index = start; index < start + needle.length; index++) marks[index] = true;
    }
    return matched;
  }

  function highlightHTML(value, query) {
    var original = Array.from(asString(value));
    if (!original.length) return "";
    var folded = original.map(function (character) { return foldSearchText(character); });
    var marks = original.map(function () { return false; });
    var normalizedQuery = foldSearchText(trimSearchText(asString(query)));
    if (!normalizedQuery) return escapeHTML(original.join(""));
    var tokens;
    try { tokens = tokenize(normalizedQuery); } catch (error) { tokens = [normalizedQuery]; }
    tokens.forEach(function (token) {
      var literal = Array.from(token);
      if (markNeedle(folded, literal, marks)) return;
      trigrams(token).forEach(function (trigram) {
        markNeedle(folded, Array.from(trigram), marks);
      });
    });
    var output = "";
    var marked = false;
    original.forEach(function (character, index) {
      if (marks[index] !== marked) {
        if (marked) output += "</span>";
        if (marks[index]) output += '<span class="search-highlight">';
        marked = marks[index];
      }
      output += escapeHTML(character);
    });
    if (marked) output += "</span>";
    return output;
  }

  // Go sorts UTF-8 strings by scalar value; JS relational operators sort UTF-16.
  function searchKeyLess(left, right) {
    if (typeof left !== "string" || typeof right !== "string") return left < right;
    var a = Array.from(left), b = Array.from(right);
    for (var index = 0; index < Math.min(a.length, b.length); index++) {
      var delta = a[index].codePointAt(0) - b[index].codePointAt(0);
      if (delta) return delta < 0;
    }
    return a.length < b.length;
  }

  function lowerBound(values, key, keyFor) {
    var low = 0;
    var high = values.length;
    while (low < high) {
      var middle = low + Math.floor((high - low) / 2);
      if (searchKeyLess(keyFor(values[middle]), key)) low = middle + 1;
      else high = middle;
    }
    return low;
  }

  function postingRoutes(routes, key, allowPrefix) {
    var start = lowerBound(routes, key, function (route) { return route.key; });
    if (start < routes.length && routes[start].key === key) return [routes[start]];
    if (!allowPrefix) return [];
    var end = start;
    while (end < routes.length && routes[end].key.indexOf(key) === 0) end++;
    return routes.slice(start, end);
  }

  function boundedDamerauLevenshtein(leftValue, rightValue, limit) {
    var left = Array.from(leftValue);
    var right = Array.from(rightValue);
    if (Math.abs(left.length - right.length) > limit) return limit + 1;
    var previousPrevious = new Array(right.length + 1).fill(0);
    var previous = Array.from({ length: right.length + 1 }, function (_, index) { return index; });
    for (var leftIndex = 1; leftIndex <= left.length; leftIndex++) {
      var current = new Array(right.length + 1).fill(0);
      current[0] = leftIndex;
      for (var rightIndex = 1; rightIndex <= right.length; rightIndex++) {
        var cost = left[leftIndex - 1] === right[rightIndex - 1] ? 0 : 1;
        current[rightIndex] = Math.min(
          previous[rightIndex] + 1,
          current[rightIndex - 1] + 1,
          previous[rightIndex - 1] + cost
        );
        if (leftIndex > 1 && rightIndex > 1 && left[leftIndex - 1] === right[rightIndex - 2] && left[leftIndex - 2] === right[rightIndex - 1]) {
          current[rightIndex] = Math.min(current[rightIndex], previousPrevious[rightIndex - 2] + 1);
        }
      }
      previousPrevious = previous;
      previous = current;
    }
    return previous[right.length];
  }

  function fuzzyPostingRoutes(routes, token) {
    var runes = Array.from(token);
    if (runes.length < 4) return [];
    var maxDistance = runes.length >= 8 ? 2 : 1;
    var first = runes[0];
    var start = lowerBound(routes, first, function (route) { return route.key; });
    var matches = [];
    for (var index = start; index < routes.length && routes[index].key.indexOf(first) === 0; index++) {
      var candidateLength = Array.from(routes[index].key).length;
      if (Math.abs(candidateLength - runes.length) > maxDistance) continue;
      var distance = boundedDamerauLevenshtein(token, routes[index].key, maxDistance);
      if (distance <= maxDistance) matches.push({ route: routes[index], distance: distance });
    }
    matches.sort(function (left, right) {
      if (left.distance !== right.distance) return left.distance - right.distance;
      return left.route.key.localeCompare(right.route.key);
    });
    return matches.slice(0, MAX_RESULTS < 8 ? MAX_RESULTS : 8).map(function (match) { return match.route; });
  }

  function union(left, right) {
    var result = [];
    var i = 0;
    var j = 0;
    while (i < left.length || j < right.length) {
      if (j === right.length || (i < left.length && left[i] < right[j])) result.push(left[i++]);
      else if (i === left.length || right[j] < left[i]) result.push(right[j++]);
      else {
        result.push(left[i]);
        i++;
        j++;
      }
    }
    return result;
  }

  function intersect(left, right) {
    var result = [];
    var i = 0;
    var j = 0;
    while (i < left.length && j < right.length) {
      if (left[i] < right[j]) i++;
      else if (right[j] < left[i]) j++;
      else {
        result.push(left[i]);
        i++;
        j++;
      }
    }
    return result;
  }

  function bytesToHex(bytes) {
    return Array.from(bytes).map(function (value) { return value.toString(16).padStart(2, "0"); }).join("");
  }

  function childURL(base, childPath) {
    if (!childPath || childPath.indexOf("search/") !== 0 || childPath.indexOf("\\") >= 0) {
      throw new Error("Invalid search child path");
    }
    var segments = childPath.split("/");
    if (segments.some(function (segment) { return !segment || segment === "." || segment === ".."; })) {
      throw new Error("Invalid search child path");
    }
    return base + segments.map(encodeURIComponent).join("/");
  }

  function validReference(reference) {
    return reference && asString(reference.path).indexOf("search/") === 0 &&
      Number.isSafeInteger(reference.length) && reference.length >= 0 &&
      /^[0-9a-f]{64}$/.test(asString(reference.sha256));
  }

  function SearchRouter(root) {
    this.childBase = root.dataset.searchChildBase || "";
    this.directoryPath = root.dataset.searchDirectoryPath || "";
    this.directoryLength = Number(root.dataset.searchDirectoryLength);
    this.directorySHA256 = root.dataset.searchDirectorySha256 || "";
    this.fallbackURL = root.dataset.searchFallbackUrl || "";
    this.globalSearch = root.dataset.searchGlobal === "true";
	this.deploymentDirectoryURL = root.dataset.searchDeploymentDirectoryUrl || "";
	this.deploymentDirectoryLength = Number(root.dataset.searchDeploymentDirectoryLength);
	this.deploymentDirectorySHA256 = root.dataset.searchDeploymentDirectorySha256 || "";
    this.contextMount = root.dataset.searchContextMount || "";
    this.contextDocument = root.dataset.searchContextDocument || "";
	this.contextCatalogID = root.dataset.searchCatalogId || "";
    this.mount = root.dataset.searchMount || "/";
    this.documentLabels = Object.create(null);
    try {
      var labels = JSON.parse(root.dataset.searchDocumentLabels || "{}");
      if (labels && typeof labels === "object" && !Array.isArray(labels)) {
        Object.keys(labels).forEach(function (key) {
          if (typeof labels[key] === "string" && labels[key].trim()) this.documentLabels[key] = labels[key].trim();
        }.bind(this));
      }
    } catch (_) {}
    this.cache = new Map();
    this.directoryPromise = null;
	this.deploymentDirectoryPromise = null;
  }

  SearchRouter.prototype.fetchVerifiedURL = function (rawURL, length, digest) {
	if (!Number.isSafeInteger(length) || length < 0 || !/^[0-9a-f]{64}$/.test(asString(digest))) {
	  return Promise.reject(new Error("Invalid verified JSON metadata"));
	}
	var url;
	try {
	  url = new URL(rawURL, window.location.origin);
	} catch (_) {
	  return Promise.reject(new Error("Invalid verified JSON URL"));
	}
	if (url.origin !== window.location.origin || url.search || url.hash) {
	  return Promise.reject(new Error("Invalid verified JSON URL"));
	}
	// Older page shells used the mutable deployment alias. Bind that request
	// to its declared digest too, so retained generations can serve its bytes.
	var legacyDirectory = "/_manja/search/directory.json";
	if (url.pathname.endsWith(legacyDirectory)) {
	  url.pathname = url.pathname.slice(0, -legacyDirectory.length) + "/_manja/search/directory-sha256-" + digest + ".json";
	}
	var key = url.pathname + ":" + digest;
	if (this.cache.has(key)) return this.cache.get(key);
	var request = fetch(url.pathname, { headers: { Accept: "application/json" } })
	  .then(function (response) {
		if (!response.ok) throw new Error("Verified JSON request failed");
		return response.arrayBuffer();
	  })
	  .then(function (buffer) {
		if (buffer.byteLength !== length) throw new Error("Verified JSON length differs");
		return crypto.subtle.digest("SHA-256", buffer).then(function (actual) {
		  if (bytesToHex(new Uint8Array(actual)) !== digest) throw new Error("Verified JSON digest differs");
		  return JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(buffer));
		});
	  });
	this.cache.set(key, request);
	request.catch(function () { this.cache.delete(key); }.bind(this));
	return request;
  };

  SearchRouter.prototype.fetchVerified = function (path, length, digest) {
    if (!validReference({ path: path, length: length, sha256: digest })) {
      return Promise.reject(new Error("Invalid search child metadata"));
    }
    var key = path + ":" + digest;
    if (this.cache.has(key)) return this.cache.get(key);
    var request = fetch(childURL(this.childBase, path), { headers: { Accept: "application/json" } })
      .then(function (response) {
        if (!response.ok) throw new Error("Search child request failed");
        return response.arrayBuffer();
      })
      .then(function (buffer) {
        if (buffer.byteLength !== length) throw new Error("Search child length differs");
        return crypto.subtle.digest("SHA-256", buffer).then(function (actual) {
          if (bytesToHex(new Uint8Array(actual)) !== digest) throw new Error("Search child digest differs");
          var text = new TextDecoder("utf-8", { fatal: true }).decode(buffer);
          return JSON.parse(text);
        });
      });
    this.cache.set(key, request);
    request.catch(function () { this.cache.delete(key); }.bind(this));
    return request;
  };

  SearchRouter.prototype.loadDirectory = function () {
    if (this.directoryPromise) return this.directoryPromise;
    this.directoryPromise = this.fetchVerified(this.directoryPath, this.directoryLength, this.directorySHA256)
      .then(function (directory) {
        if (!directory || directory.schemaVersion !== 1 || directory.searchVersion !== 1 ||
            !Array.isArray(directory.exactBuckets) || !Array.isArray(directory.tokenRoutes) ||
            !Array.isArray(directory.trigramRoutes) || !Array.isArray(directory.postingSegments) ||
            !Array.isArray(directory.trigramSegments) || !Array.isArray(directory.recordSegments) ||
            !Array.isArray(directory.ranks)) {
          throw new Error("Search directory is invalid");
        }
        return directory;
      });
    this.directoryPromise.catch(function () { this.directoryPromise = null; }.bind(this));
    return this.directoryPromise;
  };

  SearchRouter.prototype.loadDeploymentDirectory = function () {
	if (this.deploymentDirectoryPromise) return this.deploymentDirectoryPromise;
	this.deploymentDirectoryPromise = this.fetchVerifiedURL(
	  this.deploymentDirectoryURL,
	  this.deploymentDirectoryLength,
	  this.deploymentDirectorySHA256
	).then(function (directory) {
	  if (!directory || directory.schemaVersion !== 1 || directory.searchVersion !== 1 || !Array.isArray(directory.catalogs)) {
		throw new Error("Deployment search directory is invalid");
	  }
	  directory.catalogs.forEach(function (catalog) {
		if (!catalog || !asString(catalog.catalogId) || !asString(catalog.title) || !asString(catalog.mount) ||
			!asString(catalog.childBase) || !validReference({ path: catalog.directoryPath, length: catalog.directoryLength, sha256: catalog.directorySha256 }) ||
			!Array.isArray(catalog.documents)) {
		  throw new Error("Deployment search catalog is invalid");
		}
		catalog.documents.forEach(function (document) {
		  if (!document || !asString(document.key) || !asString(document.title) || !asString(document.href)) {
			throw new Error("Deployment search document is invalid");
		  }
		});
	  });
	  return directory;
	});
	this.deploymentDirectoryPromise.catch(function () { this.deploymentDirectoryPromise = null; }.bind(this));
	return this.deploymentDirectoryPromise;
  };

  SearchRouter.prototype.reserve = function (receipt, reference, includePostings) {
    if (!validReference(reference)) throw new Error("Invalid search reference");
    if (receipt.paths.has(reference.path)) return;
    if (receipt.segments + 1 > MAX_SEGMENTS || receipt.bytes + reference.length > MAX_DECODED_BYTES) {
      throw new Error("Search query is too broad");
    }
    var postings = includePostings ? Number(reference.postings) : 0;
    if (!Number.isSafeInteger(postings) || postings < 0 || receipt.postings + postings > MAX_POSTINGS) {
      throw new Error("Search query is too broad");
    }
    receipt.paths.add(reference.path);
    receipt.segments++;
    receipt.bytes += reference.length;
    receipt.postings += postings;
  };

  SearchRouter.prototype.reserveRecord = function (receipt, reference) {
    if (!validReference(reference)) throw new Error("Invalid search reference");
    if (receipt.paths.has(reference.path)) return;
    if (receipt.recordSegments + 1 > MAX_RESULTS || receipt.bytes + reference.length > MAX_DECODED_BYTES) {
      throw new Error("Search query is too broad");
    }
    receipt.paths.add(reference.path);
    receipt.segments++;
    receipt.recordSegments++;
    receipt.bytes += reference.length;
  };

  SearchRouter.prototype.loadExact = function (directory, exact, receipt) {
    return crypto.subtle.digest("SHA-256", new TextEncoder().encode(exact)).then(function (digest) {
      var digestHex = bytesToHex(new Uint8Array(digest));
      var reference = null;
      for (var length = digestHex.length; length > 0; length--) {
        var prefix = digestHex.slice(0, length);
        var index = lowerBound(directory.exactBuckets, prefix, function (bucket) { return bucket.prefix; });
        if (index < directory.exactBuckets.length && directory.exactBuckets[index].prefix === prefix) {
          reference = directory.exactBuckets[index];
          break;
        }
      }
      if (!reference) return [];
      this.reserve(receipt, reference, true);
      return this.fetchVerified(reference.path, reference.length, reference.sha256).then(function (segment) {
        if (!segment || segment.schemaVersion !== 1 || segment.searchVersion !== 1 || !Array.isArray(segment.entries)) {
          throw new Error("Exact search segment is invalid");
        }
        var entryIndex = lowerBound(segment.entries, exact, function (entry) { return entry.key; });
        if (entryIndex >= segment.entries.length || segment.entries[entryIndex].key !== exact) return [];
        return Array.isArray(segment.entries[entryIndex].matches) ? segment.entries[entryIndex].matches : [];
      });
    }.bind(this));
  };

  SearchRouter.prototype.loadPostingEntries = function (references, ordinals, receipt) {
    var ordered = Array.from(ordinals).sort(function (left, right) { return left - right; });
    var selected = ordered.map(function (ordinal) {
      if (!Number.isSafeInteger(ordinal) || ordinal < 0 || ordinal >= references.length) throw new Error("Invalid posting segment ordinal");
      var reference = references[ordinal];
      this.reserve(receipt, reference, true);
      return reference;
    }.bind(this));
    return Promise.all(selected.map(function (reference) {
      return this.fetchVerified(reference.path, reference.length, reference.sha256);
    }.bind(this))).then(function (segments) {
      var entries = new Map();
      segments.forEach(function (segment) {
        if (!segment || segment.schemaVersion !== 1 || segment.searchVersion !== 1 || !Array.isArray(segment.entries)) {
          throw new Error("Posting search segment is invalid");
        }
        segment.entries.forEach(function (entry) {
          if (!entry || !Array.isArray(entry.records)) throw new Error("Posting search entry is invalid");
          entries.set(entry.key, entry.records);
        });
      });
      return entries;
    });
  };

  SearchRouter.prototype.loadCandidates = function (directory, tokens, receipt) {
    var groups = [];
    var postingOrdinals = new Set();
    for (var tokenIndex = 0; tokenIndex < tokens.length; tokenIndex++) {
      var token = tokens[tokenIndex];
      var routes = postingRoutes(directory.tokenRoutes, token, true);
      if (routes.length) {
        groups.push({ keys: routes.map(function (route) { postingOrdinals.add(route.segment); return route.key; }), fuzzy: false });
        continue;
      }
      routes = fuzzyPostingRoutes(directory.tokenRoutes, token);
      if (!routes.length) return Promise.resolve([]);
      groups.push({ keys: routes.map(function (route) { postingOrdinals.add(route.segment); return route.key; }), fuzzy: true });
    }
    if (postingOrdinals.size > MAX_TOKEN_SEGMENTS) {
      return Promise.reject(new Error("Search query is too broad"));
    }
    return this.loadPostingEntries(directory.postingSegments, postingOrdinals, receipt).then(function (loaded) {
      var candidates = [];
      groups.forEach(function (group, groupIndex) {
        var groupCandidates = [];
        group.keys.forEach(function (key) {
          groupCandidates = union(groupCandidates, loaded.get(key) || []);
        });
        candidates = groupIndex === 0 ? groupCandidates : intersect(candidates, groupCandidates);
      });
      return candidates;
    });
  };

  SearchRouter.prototype.loadRecords = function (directory, recordIDs, receipt) {
    if (!recordIDs.length) return Promise.resolve([]);
    var indexes = new Set();
    recordIDs.forEach(function (recordID) {
      var index = lowerBound(directory.recordSegments, recordID + 1, function (reference) {
        return Number(reference.firstRecord) + Number(reference.records);
      });
      if (index >= directory.recordSegments.length || recordID < directory.recordSegments[index].firstRecord) {
        throw new Error("Invalid search record ordinal");
      }
      indexes.add(index);
    });
    var selected = Array.from(indexes).sort(function (left, right) { return left - right; }).map(function (index) {
      var reference = directory.recordSegments[index];
      this.reserveRecord(receipt, reference);
      return reference;
    }.bind(this));
    return Promise.all(selected.map(function (reference) {
      return this.fetchVerified(reference.path, reference.length, reference.sha256);
    }.bind(this))).then(function (segments) {
      var records = new Map();
      segments.forEach(function (segment) {
        if (!segment || segment.schemaVersion !== 1 || segment.searchVersion !== 1 || !Array.isArray(segment.records)) {
          throw new Error("Search record segment is invalid");
        }
        segment.records.forEach(function (record, offset) { records.set(segment.firstRecord + offset, record); });
      });
      return recordIDs.map(function (recordID) {
        if (!records.has(recordID)) throw new Error("Search record is missing");
        return records.get(recordID);
      });
    });
  };

  SearchRouter.prototype.resultHref = function (raw) {
    raw = asString(raw).trim();
    if (!raw || raw.indexOf("\\") >= 0 || /^[a-z][a-z0-9+.-]*:/i.test(raw) || raw.indexOf("//") === 0) return "";
    var parsed = new URL(raw, "https://manja.invalid/");
    var path = parsed.pathname.replace(/^\/+/, "");
    var segments = path.split("/");
    if (segments.some(function (segment) { return segment === ".."; })) return "";
    if (path.indexOf("documents/") !== 0) path = "documents/" + path;
    var prefix = this.mount === "/" ? "" : this.mount;
    return prefix + "/" + path + parsed.search + parsed.hash;
  };

  SearchRouter.prototype.searchClient = function (query) {
    var exact = normalizeExact(query);
    var receipt = { paths: new Set(), segments: 0, recordSegments: 0, bytes: 0, postings: 0 };
    return this.loadDirectory().then(function (directory) {
      return this.loadExact(directory, exact, receipt).then(function (matches) {
        var priorities = new Map();
        var exactIDs = [];
        matches.forEach(function (match) {
          var record = Number(match.record);
          exactIDs.push(record);
          var priority = Number(match.priority);
          if (!priorities.has(record) || priority < priorities.get(record)) priorities.set(record, priority);
        });
        exactIDs = Array.from(new Set(exactIDs.sort(function (a, b) { return a - b; })));
        var candidates;
        try {
          candidates = this.loadCandidates(directory, tokenize(exact), receipt);
        } catch (error) {
          candidates = exactIDs.length ? Promise.resolve([]) : Promise.reject(error);
        }
        return candidates.catch(function (error) {
          if (exactIDs.length) return [];
          throw error;
        }).then(function (tokenIDs) {
          var candidateIDs = Array.from(new Set(exactIDs.concat(tokenIDs))).sort(function (left, right) { return left - right; });
          if (candidateIDs.length > MAX_POSTINGS) {
            if (exactIDs.length) candidateIDs = exactIDs;
            else throw new Error("Search query is too broad");
          }
          candidateIDs.sort(function (left, right) {
            var leftPriority = priorities.get(left) || 0;
            var rightPriority = priorities.get(right) || 0;
            if (leftPriority !== rightPriority) {
              if (leftPriority === 0) return 1;
              if (rightPriority === 0) return -1;
              return leftPriority - rightPriority;
            }
            var leftKind = searchKindPriority(directory.ranks[left].k);
            var rightKind = searchKindPriority(directory.ranks[right].k);
            if (leftKind !== rightKind) return leftKind - rightKind;
            var leftTitle = normalizeCorpus(directory.ranks[left].t);
            var rightTitle = normalizeCorpus(directory.ranks[right].t);
            if ((leftTitle === exact) !== (rightTitle === exact)) return leftTitle === exact ? -1 : 1;
            var lengthDifference = utf8Length(directory.ranks[left].t) - utf8Length(directory.ranks[right].t);
            return lengthDifference || left - right;
          });
          return this.loadRecords(directory, candidateIDs.slice(0, MAX_RESULTS), receipt);
        }.bind(this));
      }.bind(this));
    }.bind(this)).then(function (records) {
      return records.map(function (record) {
        return {
          id: asString(record.detailId),
          title: asString(record.title),
          description: asString(record.description),
          href: this.resultHref(record.href),
          kind: asString(record.kind),
          operationId: asString(record.operationId),
          method: asString(record.method).toUpperCase(),
          path: asString(record.path),
          section: this.documentLabels[asString(record.documentKey)] || "",
        };
      }.bind(this));
    }.bind(this));
  };

  SearchRouter.prototype.searchFallback = function (query) {
    var url = new URL(this.fallbackURL, window.location.origin);
    url.searchParams.set("q", query);
    if (this.globalSearch && this.contextMount) url.searchParams.set("context_mount", this.contextMount);
    if (this.globalSearch && this.contextDocument) url.searchParams.set("context_document", this.contextDocument);
    return fetch(url.toString(), { headers: { Accept: "application/json" } }).then(function (response) {
      if (!response.ok) {
        if (response.status === 400) throw new Error("Enter a valid search query");
        if (response.status === 422) throw new Error("Search is too broad. Add another term");
        throw new Error("Search is temporarily unavailable");
      }
      return response.json();
    }).then(function (payload) {
      if (!payload || !Array.isArray(payload.results)) throw new Error("Search fallback response is invalid");
      return payload.results.slice(0, MAX_RESULTS).map(function (record) {
        return {
          id: asString(record.detailId), title: asString(record.title), description: asString(record.description),
          href: asString(record.href), kind: asString(record.kind), method: asString(record.method).toUpperCase(),
          operationId: asString(record.operationId), path: asString(record.path),
          section: this.documentLabels[asString(record.documentKey)] || asString(record.section || ""),
        };
      }.bind(this));
    }.bind(this));
  };

  function deploymentNavigationMatch(value, query) {
	var normalized = normalizeCorpus(value);
	var exact = normalizeExact(query);
	if (!normalized || !exact) return -1;
	if (normalized === exact) return 0;
	if (normalized.indexOf(exact) === 0) return 1;
	if (normalized.indexOf(exact) >= 0) return 2;
	var queryTokens = tokenize(exact);
	var valueTokens = [];
	var candidate = "";
	Array.from(normalized).some(function (character) {
	  if (/[\p{L}\p{Nd}/{}.:_-]/u.test(character)) candidate += character;
	  else if (candidate) { valueTokens.push(candidate); candidate = ""; }
	  return valueTokens.length >= 512;
	});
	if (candidate && valueTokens.length < 512) valueTokens.push(candidate);
	var allMatched = queryTokens.every(function (token) {
	  return valueTokens.some(function (candidate) {
		if (candidate.indexOf(token) === 0) return true;
		var limit = Array.from(token).length >= 8 ? 2 : 1;
		return Array.from(token).length >= 4 && boundedDamerauLevenshtein(token, candidate, limit) <= limit;
	  });
	});
	return allMatched ? 3 : -1;
  }

  function deploymentResultQuality(item, query) {
	var fields = [item.title, item.operationId, item.path];
	var best = 4;
	fields.forEach(function (field) {
	  var quality = deploymentNavigationMatch(field, query);
	  if (quality >= 0 && quality < best) best = quality;
	});
	if (best < 4) return best;
	return deploymentNavigationMatch([item.description, item.section].join(" "), query) >= 0 ? 4 : 5;
  }

  function mapConcurrent(values, limit, worker) {
	var results = new Array(values.length);
	var next = 0;
	function run() {
	  var index = next++;
	  if (index >= values.length) return Promise.resolve();
	  return Promise.resolve(worker(values[index], index)).then(function (result) {
		results[index] = result;
	  }).then(run);
	}
	var runners = [];
	for (var index = 0; index < Math.min(limit, values.length); index++) runners.push(run());
	return Promise.all(runners).then(function () { return results; });
  }

  SearchRouter.prototype.catalogRouter = function (catalog) {
	var labels = Object.create(null);
	catalog.documents.forEach(function (document) { labels[document.key] = document.title; });
	var root = { dataset: {
	  searchChildBase: catalog.childBase,
	  searchDirectoryPath: catalog.directoryPath,
	  searchDirectoryLength: String(catalog.directoryLength),
	  searchDirectorySha256: catalog.directorySha256,
	  searchGlobal: "false",
	  searchMount: catalog.mount,
	  searchDocumentLabels: JSON.stringify(labels)
	} };
	var router = new SearchRouter(root);
	// Reuse verified segment bytes across catalog routers and repeated queries.
	router.cache = this.cache;
	return router;
  };

  SearchRouter.prototype.searchDeployment = function (query) {
	return this.loadDeploymentDirectory().then(function (directory) {
	  return mapConcurrent(directory.catalogs, 4, function (catalog) {
		return this.catalogRouter(catalog).searchClient(query).then(function (items) {
		  return { catalog: catalog, items: items, failed: false };
		}).catch(function () {
		  return { catalog: catalog, items: [], failed: true };
		});
	  }.bind(this));
	}.bind(this)).then(function (catalogResults) {
	  var merged = [];
	  var failures = 0;
	  catalogResults.forEach(function (result) {
		var catalog = result.catalog;
		if (result.failed) failures++;
		var catalogQuality = deploymentNavigationMatch([catalog.title, catalog.catalogId].join(" "), query);
		if (catalogQuality >= 0) {
		  merged.push({
			id: "catalog-" + catalog.catalogId, title: catalog.title, description: "API catalog",
			href: catalog.mount === "/" ? "/" : catalog.mount + "/", kind: "catalog", method: "", operationId: "", path: "", section: "Catalogs",
			_quality: catalogQuality, _context: catalog.catalogId === this.contextCatalogID ? 0 : 1
		  });
		}
		catalog.documents.forEach(function (document) {
		  var quality = deploymentNavigationMatch([document.title, document.key].join(" "), query);
		  if (quality < 0 && asString(document.description)) {
			quality = deploymentNavigationMatch(document.description, query) >= 0 ? 4 : -1;
		  }
		  if (quality < 0) return;
		  merged.push({
			id: "document-" + catalog.catalogId + "-" + document.key, title: document.title, description: asString(document.description) || catalog.title,
			href: document.href, kind: "document", method: "", operationId: "", path: "", section: catalog.title,
			_quality: quality, _context: catalog.catalogId === this.contextCatalogID && document.key === this.contextDocument ? 0 : 1
		  });
		}.bind(this));
		result.items.forEach(function (item) {
		  item.section = item.section ? catalog.title + " · " + item.section : catalog.title;
		  item._quality = deploymentResultQuality(item, query);
		  item._context = catalog.catalogId === this.contextCatalogID && (!this.contextDocument || item.href.indexOf("/documents/" + encodeURIComponent(this.contextDocument) + "/") >= 0) ? 0 : 1;
		  merged.push(item);
		}.bind(this));
	  }.bind(this));
	  var seen = new Set();
	  merged = merged.filter(function (item) {
		var key = item.kind + "\u0000" + item.href;
		if (seen.has(key)) return false;
		seen.add(key);
		return true;
	  });
	  merged.sort(function (left, right) {
		if (left._quality !== right._quality) return left._quality - right._quality;
		if (left._context !== right._context) return left._context - right._context;
		var kind = searchKindPriority(left.kind) - searchKindPriority(right.kind);
		if (kind) return kind;
		return normalizeCorpus(left.title).localeCompare(normalizeCorpus(right.title)) || left.href.localeCompare(right.href);
	  });
	  return { items: merged.slice(0, MAX_RESULTS), failures: failures, catalogs: catalogResults.length };
	}.bind(this));
  };

  SearchRouter.prototype.search = function (query) {
	if (this.globalSearch && this.deploymentDirectoryURL) {
	  return this.searchDeployment(query).then(function (result) {
		var source = "Deployment search";
		if (result.failures) source += " · " + result.failures + " catalog" + (result.failures === 1 ? "" : "s") + " unavailable";
		return { items: result.items, source: source };
	  });
	}
    if (this.globalSearch) {
      return this.searchFallback(query).then(function (items) {
        return { items: items, source: "Global search" };
      });
    }
    return this.searchClient(query).then(function (items) {
      return { items: items, source: "Browser index" };
    }).catch(function () {
      return this.searchFallback(query).then(function (items) {
        return { items: items, source: "Server fallback" };
      });
    }.bind(this));
  };

  function safePageHref(value, mount) {
    try {
      var url = new URL(asString(value), window.location.origin);
      if (url.origin !== window.location.origin) return "";
      if (mount !== "/" && url.pathname !== mount && url.pathname.indexOf(mount + "/") !== 0) return "";
      return url.pathname + url.search + url.hash;
    } catch (error) {
      return "";
    }
  }

  function normalizeDisplayItem(raw, mount) {
    raw = raw || {};
    var href = safePageHref(raw.href, mount);
    if (!href) return null;
    return {
      id: asString(raw.id || raw.detailId).slice(0, 160),
      title: asString(raw.title).slice(0, 200),
      description: asString(raw.description).slice(0, 320),
      href: href.slice(0, 2048),
      kind: asString(raw.kind).slice(0, 32),
      operationId: asString(raw.operationId).slice(0, 256),
      method: asString(raw.method).toUpperCase().slice(0, 16),
      path: asString(raw.path).slice(0, 512),
      section: asString(raw.section).slice(0, 160),
    };
  }

  function groupSearchItems(items) {
    var order = [];
    var groups = new Map();
    items.forEach(function (item) {
      var labels = { catalog: "Catalogs", document: "Specs", operation: "Operations", schema: "Schemas" };
      var label = labels[item.kind.toLowerCase()] || "Other results";
      if (!groups.has(label)) {
        groups.set(label, []);
        order.push(label);
      }
      groups.get(label).push(item);
    });
    var grouped = [];
    order.forEach(function (label) {
      var values = groups.get(label);
      values.forEach(function (item, index) {
        grouped.push(Object.assign({}, item, {
          groupStart: index === 0,
          groupLabel: label,
          groupCount: values.length,
        }));
      });
    });
    return grouped;
  }

  window.ManjaCatalogSearchRouter = {
	create: function (root) { return new SearchRouter(root); },
	deploymentNavigationMatch: deploymentNavigationMatch,
  };

	window.manjaCatalogSearch = function (root) {
	  var router = window.ManjaCatalogSearchRouter.create(root);
	  var mount = root.dataset.searchMount || "/";
	  var scopeLabel = root.dataset.searchScopeLabel || "";
    var storageKey = "manja.catalog.recent.v1:" + (root.dataset.searchCatalogId || "catalog");
    return {
      open: false,
      query: "",
      recent: [],
      results: [],
      activeIndex: 0,
      loading: false,
      error: "",
      sourceLabel: "",
      timer: null,
      generation: 0,
      previousFocus: null,
      init: function () {
        this.readRecent();
        var payload = document.getElementById("catalog-search-current-visit");
        if (!payload) return;
        try { this.remember(JSON.parse(payload.textContent)); } catch (error) {}
      },
      readRecent: function () {
        var values = [];
        try {
          var parsed = JSON.parse(localStorage.getItem(storageKey) || "[]");
          if (Array.isArray(parsed)) values = parsed;
        } catch (error) {}
        this.recent = values.map(function (item) { return normalizeDisplayItem(item, mount); }).filter(Boolean).slice(0, MAX_RECENT);
      },
      remember: function (raw) {
        var item = normalizeDisplayItem(raw, mount);
        if (!item || !item.title) return;
        this.recent = [item].concat(this.recent.filter(function (existing) { return existing.href !== item.href; })).slice(0, MAX_RECENT);
        try { localStorage.setItem(storageKey, JSON.stringify(this.recent)); } catch (error) {}
      },
      handleWindowKey: function (event) {
        if (event.defaultPrevented) return;
        if ((event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey && asString(event.key).toLowerCase() === "k") {
          event.preventDefault();
          if (!this.open) this.openSearch(true);
          else this.$nextTick(function () { this.focusInput(true); }.bind(this));
          return;
        }
        if (this.open && event.key === "Escape") {
          event.preventDefault();
          this.closeSearch();
        }
      },
      focusInput: function (focusVisible) {
        if (!this.$refs.input) return;
        var input = this.$refs.input;
        if (focusVisible) {
          input.dataset.keyboardFocus = "true";
          var clearKeyboardFocus = function () { delete input.dataset.keyboardFocus; };
          input.addEventListener("pointerdown", clearKeyboardFocus, { once: true });
          input.addEventListener("blur", clearKeyboardFocus, { once: true });
        } else {
          delete input.dataset.keyboardFocus;
        }
        if (focusVisible) {
          try {
            input.focus({ focusVisible: true });
            return;
          } catch (error) {}
        }
        input.focus();
      },
      openSearch: function (focusVisible) {
        var focus = document.activeElement;
        if (focus && focus.closest && focus.closest("#catalog-navigation")) {
          focus = document.querySelector('[aria-controls="catalog-navigation"]') || focus;
        }
        this.previousFocus = focus;
        this.open = true;
        this.query = "";
        this.results = [];
        this.error = "";
        this.sourceLabel = "";
        this.activeIndex = 0;
        this.readRecent();
        this.resetResultsScroll();
        window.dispatchEvent(new CustomEvent("goshtoso-search-open", { detail: { id: "catalog-search" } }));
        this.$nextTick(function () { this.focusInput(Boolean(focusVisible)); }.bind(this));
      },
      closeSearch: function (restoreFocus) {
        if (restoreFocus === undefined) restoreFocus = true;
        if (this.timer) clearTimeout(this.timer);
        this.timer = null;
        this.generation++;
        this.open = false;
        this.query = "";
        this.results = [];
        this.loading = false;
        this.error = "";
        this.sourceLabel = "";
        window.dispatchEvent(new CustomEvent("goshtoso-search-close", { detail: { id: "catalog-search" } }));
        var focus = this.previousFocus;
        if (restoreFocus) this.$nextTick(function () { if (focus && focus.focus) focus.focus(); });
      },
      clearQuery: function () {
        if (this.timer) clearTimeout(this.timer);
        this.timer = null;
        this.generation++;
        this.query = "";
        this.results = [];
        this.loading = false;
        this.error = "";
        this.sourceLabel = "";
        this.activeIndex = 0;
        this.resetResultsScroll();
        this.$nextTick(function () { this.$refs.input.focus(); }.bind(this));
      },
      resetResultsScroll: function () {
        this.$nextTick(function () {
          if (this.$refs.results) this.$refs.results.scrollTop = 0;
        }.bind(this));
      },
      scrollActiveIntoView: function () {
        this.$nextTick(function () {
          var option = document.getElementById(this.optionID(this.activeIndex));
          var results = this.$refs.results;
          if (!option || !results || !option.scrollIntoView) return;
          option.scrollIntoView({ block: "nearest" });
          var bounds = results.getBoundingClientRect();
          var top = bounds.top;
          var bottom = bounds.bottom;
          var footer = results.querySelector("[data-catalog-search-footer]");
          if (footer) bottom = Math.min(bottom, footer.getBoundingClientRect().top);
          results.querySelectorAll("[data-catalog-search-group]").forEach(function (group) {
            if (getComputedStyle(group).display === "none") return;
            var groupBounds = group.getBoundingClientRect();
            if (groupBounds.top <= top + 1 && groupBounds.bottom > top) top = Math.max(top, groupBounds.bottom);
          });
          var optionBounds = option.getBoundingClientRect();
          if (optionBounds.top < top) results.scrollTop -= top - optionBounds.top;
          else if (optionBounds.bottom > bottom) results.scrollTop += optionBounds.bottom - bottom;
        }.bind(this));
      },
      queueSearch: function () {
        if (this.timer) clearTimeout(this.timer);
        this.timer = null;
        this.generation++;
        this.activeIndex = 0;
        this.error = "";
        this.sourceLabel = "";
        this.resetResultsScroll();
        if (!this.query.trim()) {
          this.results = [];
          this.loading = false;
          return;
        }
        this.loading = true;
        var generation = this.generation;
        this.timer = setTimeout(function () { this.runSearch(generation); }.bind(this), 100);
      },
      runSearch: function (generation) {
        var query = this.query;
        router.search(query).then(function (response) {
          if (generation !== this.generation || query !== this.query) return;
          this.results = groupSearchItems(response.items.map(function (item) { return normalizeDisplayItem(item, mount); }).filter(Boolean));
          this.sourceLabel = scopeLabel ? response.source + " · " + scopeLabel : response.source;
          this.loading = false;
          this.resetResultsScroll();
        }.bind(this)).catch(function (error) {
          if (generation !== this.generation || query !== this.query) return;
          if (window.console && typeof window.console.warn === "function") {
            window.console.warn("Manja search failed:", error);
          }
          this.results = [];
          this.sourceLabel = "";
          this.loading = false;
          this.error = error && error.message === "Search query is too broad"
            ? "Search is too broad. Add more detail to your query."
            : "Search is unavailable. Reload the page and try again.";
        }.bind(this));
      },
      visibleItems: function () { return this.query.trim() ? this.results : this.recent; },
      highlight: function (value) { return highlightHTML(value, this.query); },
      optionID: function (index) { return "catalog-search-option-" + index; },
      activeOptionID: function () {
        return this.visibleItems().length ? this.optionID(this.activeIndex) : null;
      },
      move: function (delta) {
        var values = this.visibleItems();
        if (!values.length) return;
        this.activeIndex = (this.activeIndex + delta + values.length) % values.length;
        this.scrollActiveIntoView();
      },
      moveTo: function (index) {
        var values = this.visibleItems();
        if (!values.length) return;
        this.activeIndex = Math.max(0, Math.min(index, values.length - 1));
        this.scrollActiveIntoView();
      },
      moveToEnd: function () {
        this.moveTo(this.visibleItems().length - 1);
      },
      choose: function () {
        var values = this.visibleItems();
        if (values.length) this.select(values[this.activeIndex]);
      },
      select: function (item) {
        var href = item && safePageHref(item.href, mount);
        if (!href) return;
        this.remember(item);
        var navigate = window.ManjaLocalDocsEnhancer && window.ManjaLocalDocsEnhancer.navigate;
        if (typeof navigate === "function") {
          var pending;
          try { pending = navigate(href); } catch (error) { pending = null; }
          if (pending !== null && pending !== undefined) {
            this.closeSearch(false);
            Promise.resolve(pending).catch(function () { window.location.assign(href); });
            return;
          }
        }
        window.location.assign(href);
      },
    };
  };
})();
