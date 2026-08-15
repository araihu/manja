(function (scope) {
  "use strict";

  var publications = Object.create(null);

  function sameOriginPath(value) {
    if (typeof value !== "string" || value.charAt(0) !== "/" || value.indexOf("\\") !== -1 || value.indexOf("%") !== -1 || value.indexOf("?") !== -1 || value.indexOf("#") !== -1) {
      return false;
    }
    var parsed;
    try {
      parsed = new URL(value, scope.location.origin);
    } catch (_) {
      return false;
    }
    return parsed.origin === scope.location.origin && parsed.pathname === value && parsed.search === "" && parsed.hash === "";
  }

  function validPublicationBase(value) {
    if (value === "/") {
      return true;
    }
    return typeof value === "string" && value.length >= 3 && value.charAt(0) === "/" && value.charAt(value.length - 1) === "/" && value.indexOf("//") === -1 && sameOriginPath(value);
  }

  function validIdentity(value) {
    return typeof value === "string" && value.length > 0 && value === value.trim() && !/[\u0000-\u001f\u007f]/.test(value);
  }

  function validDescriptor(descriptor) {
    if (!descriptor || typeof descriptor !== "object" || descriptor.schemaVersion !== 1 || !validIdentity(descriptor.catalogId) || !/^[a-z0-9][a-z0-9._-]{0,63}$/.test(descriptor.publicationKey) || !validIdentity(descriptor.revisionId) || descriptor.projectionFormat !== "projection-v2" || !/^[0-9a-f]{64}$/.test(descriptor.projectionDigest) || descriptor.snapshotId !== "snapshot-sha256-" + descriptor.projectionDigest || !validPublicationBase(descriptor.publicationBase)) {
      return false;
    }
    if (descriptor.public === false || descriptor.anonymous === false || descriptor.private === true || descriptor.disabled === true) {
      return false;
    }
    var base = descriptor.publicationBase + "snapshots/" + descriptor.snapshotId + "/";
    return descriptor.projectionManifestUrl === base + "manifest.json" && sameOriginPath(descriptor.projectionManifestUrl) && descriptor.catalogUrl === base + "catalog.json" && sameOriginPath(descriptor.catalogUrl) && descriptor.searchDataBase === base + "search-data/" && sameOriginPath(descriptor.searchDataBase) && descriptor.projectionDataBase === base + "projection-data/" && sameOriginPath(descriptor.projectionDataBase);
  }

  function reservedPath(pathname) {
    return pathname === "/manage" || pathname.indexOf("/manage/") === 0 || pathname === "/api" || pathname.indexOf("/api/") === 0;
  }

  function canonicalPath(pathname) {
    if (typeof pathname !== "string" || pathname.charAt(0) !== "/" || pathname.indexOf("\\") !== -1 || pathname.indexOf("%") !== -1 || pathname.indexOf("?") !== -1 || pathname.indexOf("#") !== -1) {
      return false;
    }
    if (pathname === "/") {
      return true;
    }
    var segments = pathname.split("/");
    for (var index = 1; index < segments.length; index += 1) {
      if (segments[index] === "." || segments[index] === ".." || segments[index] === "" && index !== segments.length - 1) {
        return false;
      }
    }
    return true;
  }

  function allows(request, descriptor) {
    if (!request || request.method !== "GET" || !descriptor) {
      return false;
    }
    var url;
    try {
      url = new URL(request.url);
    } catch (_) {
      return false;
    }
    if (url.origin !== scope.location.origin || url.search !== "" || url.hash !== "" || !canonicalPath(url.pathname) || reservedPath(url.pathname)) {
      return false;
    }
    var base = descriptor.publicationBase;
    var root = base === "/" ? "/" : base.slice(0, -1);
    if (url.pathname !== root && url.pathname !== base && url.pathname.indexOf(base) !== 0) {
      return false;
    }
    var routes = [descriptor.projectionManifestUrl, descriptor.catalogUrl];
    if (routes.some(function (route) { return url.pathname === route; })) {
      return true;
    }
    return url.pathname === root || url.pathname === base;
  }

  scope.addEventListener("message", function (event) {
    var message = event && event.data;
    if (!message || message.type !== "manja:configure" || !validDescriptor(message.descriptor)) {
      return;
    }
    publications[message.descriptor.publicationKey] = message.descriptor;
  });

  scope.addEventListener("fetch", function (event) {
    var request = event.request;
    var descriptor;
    var keys = Object.keys(publications);
    for (var index = 0; index < keys.length; index += 1) {
      if (allows(request, publications[keys[index]])) {
        descriptor = publications[keys[index]];
        break;
      }
    }
    if (!descriptor) {
      return;
    }
    event.respondWith(fetch(request));
  });
}(self));
