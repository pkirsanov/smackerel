// Spec 073 SCOPE-073-05 — wiki landing.
//
// BUG-080-001 SCOPE-04 / SCN-080-001-04: the landing page reads the ONE
// published aggregate so a deliberately disabled deployment stops
// advertising a Graph journey. Previously the subtitle and all four
// section links were static, so a disabled capability still looked
// available and every link led to a dead end.
import {
  readGraphActivation,
  activationDisablesJourney,
  renderReadState,
  STATE_DISABLED,
} from "/pwa/wiki_state.js";

export const WIKI_SECTIONS = ["topics", "people", "places", "time"];

async function initLanding() {
  const activation = await readGraphActivation();
  if (activationDisablesJourney(activation)) {
    const subtitle = document.getElementById("wiki-landing-subtitle");
    const nav = document.getElementById("wiki-landing-nav");
    // Remove the navigation outright rather than disabling it: an
    // aria-disabled link is still reachable by keyboard and would still
    // claim a section that cannot load.
    if (nav) nav.remove();
    if (subtitle) subtitle.textContent = "Knowledge graph browsing is turned off for this deployment.";
    renderReadState(document.getElementById("wiki-landing-status"), STATE_DISABLED, {});
    document.documentElement.setAttribute("data-graph-activation", "policy_disabled");
  }
  document.documentElement.setAttribute("data-wiki-ready", "true");
}

initLanding();
