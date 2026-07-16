"""BUG-059-001 — structural guard for the gkeepapi live-mode build pin.

The Google Keep live-mode consumer (``ml/app/keep_bridge.py``) performs a LAZY
``import gkeepapi`` inside ``authenticate()``. The mock-based unit suite never
executes that import (``test_keep.py`` pre-seeds ``bridge._keep_session`` and
patches ``authenticate``), so a missing build-manifest pin is invisible to both
the image build and the tests. This guard closes that blind spot by asserting,
at the TEXT level, that an exact ``gkeepapi==`` pin is present on the build
surfaces that actually ship the dependency.

It deliberately does NOT ``import gkeepapi``: importing the library would couple
this test to the very dependency under question and re-introduce the same blind
spot in reverse. Reading the manifest text is environment-independent and fails
precisely when the pin is removed (the exact reintroduction of BUG-059-001),
which is the adversarial regression the bug contract requires.
"""

import os
import re

_HERE = os.path.dirname(os.path.abspath(__file__))
# ml/tests/ -> ml/
_REQUIREMENTS = os.path.join(_HERE, "..", "requirements.txt")
_PYPROJECT = os.path.join(_HERE, "..", "pyproject.toml")

# An exact pin is ``gkeepapi==<version>`` at the start of a (non-comment) line.
# The optional quote handles the TOML array form (``"gkeepapi==0.17.1"``) as
# well as the bare requirements.txt form. The anchored ``==`` is what
# distinguishes an exact pin from a floated ``>=`` range.
_EXACT_PIN = re.compile(r'^\s*["\']?gkeepapi==\d', re.MULTILINE)
_TRANSFORMERS_FIXED_VERSION = "5.5.0"


def _read(path: str) -> str:
    with open(path, encoding="utf-8") as handle:
        return handle.read()


def _strip_comments(manifest_text: str) -> str:
    """Drop ``#`` comment lines so a gkeepapi mention in prose can never pass."""
    return "\n".join(line for line in manifest_text.splitlines() if not line.lstrip().startswith("#"))


def _has_exact_gkeepapi_pin(manifest_text: str) -> bool:
    """Return True iff an exact ``gkeepapi==<version>`` pin is present."""
    return bool(_EXACT_PIN.search(_strip_comments(manifest_text)))


def _exact_requirement_versions(manifest_text: str, package_name: str) -> list[str]:
    """Return active exact-pin versions for one normalized package name."""
    normalized_target = package_name.lower().replace("_", "-")
    versions: list[str] = []
    for raw_line in manifest_text.splitlines():
        active = raw_line.split("#", 1)[0].strip().strip("\"'").rstrip(",")
        if not active or "==" not in active:
            continue
        name, version = (part.strip() for part in active.split("==", 1))
        if name.lower().replace("_", "-") == normalized_target and version:
            versions.append(version)
    return versions


def _transformers_pin_is_fixed(manifest_text: str) -> bool:
    """Require exactly one active Transformers pin at the fixed release."""
    return _exact_requirement_versions(manifest_text, "transformers") == [_TRANSFORMERS_FIXED_VERSION]


def test_gkeepapi_pinned_in_requirements():
    """ml/requirements.txt (the Dockerfile-installed lock) carries an exact pin."""
    assert _has_exact_gkeepapi_pin(_read(_REQUIREMENTS)), (
        "BUG-059-001 regression: ml/requirements.txt is missing an exact "
        "`gkeepapi==` pin. The smackerel-ml image would not contain gkeepapi "
        "and live-mode authenticate() would raise "
        "RuntimeError('gkeepapi is not installed')."
    )


def test_gkeepapi_pinned_in_pyproject():
    """ml/pyproject.toml (the SST dependency source) declares the same exact pin."""
    assert _has_exact_gkeepapi_pin(_read(_PYPROJECT)), (
        "BUG-059-001 regression: ml/pyproject.toml is missing an exact "
        "`gkeepapi==` pin. The build-manifest single source of truth must "
        "declare the live-mode dependency."
    )


def test_gkeepapi_pin_removal_fails_red():
    """Adversarial: removing the pin from the lock is caught (non-tautological).

    Proves the guard discriminates present-vs-absent. We take the REAL
    ``requirements.txt`` text, strip the ``gkeepapi==`` line, and assert the
    detector then reports the pin as ABSENT. If the detector were tautological
    (e.g. it ignored its input and always returned True), this test would fail
    RED — that is exactly the protection BUG-059-001 needs.
    """
    real = _read(_REQUIREMENTS)
    assert _has_exact_gkeepapi_pin(real), "precondition: real lock must carry the pin"

    without_pin = "\n".join(line for line in real.splitlines() if not line.lstrip().startswith("gkeepapi=="))
    assert not _has_exact_gkeepapi_pin(without_pin), (
        "guard is tautological: it reported the pin present even after the "
        "gkeepapi== line was removed — it would NOT catch BUG-059-001 "
        "reintroduction."
    )


def test_detector_rejects_floated_range():
    """A floated ``gkeepapi>=...`` range must NOT satisfy the exact-pin guard.

    For a reverse-engineered library Google periodically breaks, only a
    deliberate exact pin is acceptable (design.md Q2). This proves the guard
    enforces ``==`` rather than merely detecting that gkeepapi is mentioned.
    """
    assert not _has_exact_gkeepapi_pin("gkeepapi>=0.17.1\n")
    assert _has_exact_gkeepapi_pin("gkeepapi==0.17.1\n")
    assert _has_exact_gkeepapi_pin('    "gkeepapi==0.17.1",\n')


def test_transformers_vulnerable_5_3_0_pin_is_rejected():
    """Adversarial: the invalid pre-fix production lock cannot pass."""
    assert not _transformers_pin_is_fixed("transformers==5.3.0\n")
    assert not _transformers_pin_is_fixed("sentence-transformers==5.6.0\n")


def test_transformers_patched_5_5_0_and_live_lock_pass():
    """The patched fixture and live production lock carry one exact fixed pin."""
    assert _transformers_pin_is_fixed("transformers==5.5.0\n")
    live_lock = _read(_REQUIREMENTS)
    assert _transformers_pin_is_fixed(live_lock)
    assert _exact_requirement_versions(live_lock, "sentence-transformers") == ["5.6.0"]
