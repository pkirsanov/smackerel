// Spec 091 SCOPE-02 — progressive enhancement only.
// The /register page is fully functional without JS; this file just
// focuses the username field on load for keyboard ergonomics (mirrors
// login.js focusing the token field). All validation is server-side.
(function () {
  "use strict";
  var f = document.getElementById("register-username");
  if (f && typeof f.focus === "function") {
    f.focus();
  }
})();
