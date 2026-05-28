// Spec 057 Scope 1 — progressive enhancement only.
// Page is fully functional without JS; this file just focuses the
// token field on load for keyboard ergonomics.
(function () {
  "use strict";
  var f = document.getElementById("token-field");
  if (f && typeof f.focus === "function") {
    f.focus();
  }
})();
