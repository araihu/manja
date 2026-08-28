(function () {
  "use strict";

  if (window.ManjaCatalogSearchRouter && window.manjaCatalogSearch) return;

  var MAX_SEGMENTS = 16;
  var MAX_TOKEN_SEGMENTS = 8;
  var MAX_TRIGRAM_SEGMENTS = 4;
  var MAX_DECODED_BYTES = 2 << 20;
  var MAX_POSTINGS = 10000;
  var MAX_RESULTS = 20;
  var MAX_TOKENS = 8;
  var MAX_RECENT = 6;

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
  document.addEventListener("htmx:afterSettle", function (event) {
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

  function normalizeExact(input) {
    var value = asString(input);
    if (utf8Length(value) > 256 || /[\u0000-\u001f\u007f-\u009f]/.test(value)) {
      throw new Error("Invalid search query");
    }
    if (/[^\x20-\x7e]/.test(value)) throw new Error("Server normalization required");
    value = value.normalize("NFKC").trim().toLowerCase();
    if (!value || utf8Length(value) > 256 || Array.from(value).length > 128) {
      throw new Error("Invalid search query");
    }
    return value;
  }

  function tokenize(value) {
    var tokens = [];
    var token = "";
    Array.from(value).forEach(function (character) {
      if (/[\p{L}\p{N}/{}.:_-]/u.test(character)) token += character;
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
    var folded = original.map(function (character) { return character.normalize("NFKC").toLowerCase(); });
    var marks = original.map(function () { return false; });
    var normalizedQuery = asString(query).normalize("NFKC").trim().toLowerCase();
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

  function lowerBound(values, key, keyFor) {
    var low = 0;
    var high = values.length;
    while (low < high) {
      var middle = low + Math.floor((high - low) / 2);
      if (keyFor(values[middle]) < key) low = middle + 1;
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
    this.contextMount = root.dataset.searchContextMount || "";
    this.contextDocument = root.dataset.searchContextDocument || "";
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
  }

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
    var trigramOrdinals = new Set();
    for (var tokenIndex = 0; tokenIndex < tokens.length; tokenIndex++) {
      var token = tokens[tokenIndex];
      var routes = postingRoutes(directory.tokenRoutes, token, true);
      if (routes.length) {
        groups.push({ keys: routes.map(function (route) { postingOrdinals.add(route.segment); return route.key; }), fuzzy: false });
        continue;
      }
      var keys = [];
      trigrams(token).forEach(function (trigram) {
        var matched = postingRoutes(directory.trigramRoutes, trigram, false);
        if (matched.length) {
          keys.push(matched[0].key);
          trigramOrdinals.add(matched[0].segment);
        }
      });
      if (!keys.length) return Promise.resolve([]);
      groups.push({ keys: keys, fuzzy: true });
    }
    if (postingOrdinals.size > MAX_TOKEN_SEGMENTS || trigramOrdinals.size > MAX_TRIGRAM_SEGMENTS) {
      return Promise.reject(new Error("Search query is too broad"));
    }
    return Promise.all([
      this.loadPostingEntries(directory.postingSegments, postingOrdinals, receipt),
      this.loadPostingEntries(directory.trigramSegments, trigramOrdinals, receipt),
    ]).then(function (loaded) {
      var candidates = [];
      groups.forEach(function (group, groupIndex) {
        var entries = group.fuzzy ? loaded[1] : loaded[0];
        var groupCandidates = [];
        group.keys.forEach(function (key, keyIndex) {
          var records = entries.get(key) || [];
          groupCandidates = group.fuzzy && keyIndex > 0 ? intersect(groupCandidates, records) : union(groupCandidates, records);
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
      this.reserve(receipt, reference, false);
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
    var receipt = { paths: new Set(), segments: 0, bytes: 0, postings: 0 };
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
            var leftKind = searchKindPriority(directory.ranks[left].k);
            var rightKind = searchKindPriority(directory.ranks[right].k);
            if (leftKind !== rightKind) return leftKind - rightKind;
            var leftPriority = priorities.get(left) || 0;
            var rightPriority = priorities.get(right) || 0;
            if (leftPriority !== rightPriority) {
              if (leftPriority === 0) return 1;
              if (rightPriority === 0) return -1;
              return leftPriority - rightPriority;
            }
            var leftTitle = normalizeExact(directory.ranks[left].t);
            var rightTitle = normalizeExact(directory.ranks[right].t);
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
      if (!response.ok) throw new Error("Search is temporarily unavailable");
      return response.json();
    }).then(function (payload) {
      if (!payload || !Array.isArray(payload.results)) throw new Error("Search fallback response is invalid");
      return payload.results.slice(0, MAX_RESULTS).map(function (record) {
        return {
          id: asString(record.detailId), title: asString(record.title), description: asString(record.description),
          href: asString(record.href), kind: asString(record.kind), method: asString(record.method).toUpperCase(),
          path: asString(record.path), section: this.documentLabels[asString(record.documentKey)] || asString(record.section || ""),
        };
      }.bind(this));
    }.bind(this));
  };

  SearchRouter.prototype.search = function (query) {
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
      method: asString(raw.method).toUpperCase().slice(0, 16),
      path: asString(raw.path).slice(0, 512),
      section: asString(raw.section).slice(0, 160),
    };
  }

  function groupSearchItems(items) {
    var order = [];
    var groups = new Map();
    items.forEach(function (item) {
      var labels = { operation: "Operations", schema: "Schemas", document: "Documents" };
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
          if (!this.open) this.openSearch();
          return;
        }
        if (this.open && event.key === "Escape") {
          event.preventDefault();
          this.closeSearch();
        }
      },
      openSearch: function () {
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
        this.$nextTick(function () { if (this.$refs.input) this.$refs.input.focus(); }.bind(this));
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
          this.results = [];
          this.sourceLabel = "";
          this.loading = false;
          this.error = error && error.message ? error.message : "Search is temporarily unavailable";
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
