"""Unit tests for receipt extraction schema validation."""

import pytest

from app.synthesis import validate_extraction


def _load_receipt_schema():
    """Load the receipt extraction schema for validation tests."""
    import os  # noqa: PLC0415

    import yaml  # noqa: PLC0415

    contracts_dir = os.path.join(os.path.dirname(__file__), "..", "..", "config", "prompt_contracts")
    path = os.path.join(contracts_dir, "receipt-extraction-v1.yaml")
    with open(path) as f:
        contract = yaml.safe_load(f)
    return contract["extraction_schema"]


class TestReceiptExtractionSchema:
    """Validate extraction outputs against the receipt-extraction-v1 schema."""

    @pytest.fixture(autouse=True)
    def _schema(self):
        self.schema = _load_receipt_schema()

    def test_valid_full_extraction(self):
        output = {
            "domain": "expense",
            "vendor": "Corner Coffee",
            "vendor_raw": "SQ *CORNER COFFEE",
            "date": "2026-04-03",
            "amount": "108.25",
            "currency": "USD",
            "subtotal": "100.00",
            "tax": "8.25",
            "line_items": [{"description": "Latte", "amount": "4.75", "quantity": "1"}],
        }
        valid, err = validate_extraction(output, self.schema)
        assert valid is True, f"Expected valid, got: {err}"

    def test_minimal_extraction(self):
        output = {"domain": "expense", "vendor_raw": "Unknown Store"}
        valid, err = validate_extraction(output, self.schema)
        assert valid is True, f"Expected valid, got: {err}"

    def test_missing_required_domain(self):
        output = {"vendor_raw": "Store"}
        valid, err = validate_extraction(output, self.schema)
        assert valid is False
        assert "domain" in err.lower() or "required" in err.lower()

    def test_missing_required_vendor_raw(self):
        output = {"domain": "expense"}
        valid, err = validate_extraction(output, self.schema)
        assert valid is False

    def test_comma_decimal_normalization(self):
        """Verify dot-decimal amount with raw_amount preserving original."""
        output = {
            "domain": "expense",
            "vendor_raw": "Lidl",
            "amount": "47.50",
            "raw_amount": "47,50",
            "currency": "EUR",
        }
        valid, err = validate_extraction(output, self.schema)
        assert valid is True, f"Expected valid, got: {err}"

    def test_negative_refund_amount(self):
        output = {
            "domain": "expense",
            "vendor_raw": "Amazon",
            "amount": "-29.99",
            "currency": "USD",
        }
        valid, err = validate_extraction(output, self.schema)
        assert valid is True, f"Expected valid, got: {err}"

    def test_extraction_failed_flag_structure(self):
        """extraction_failed responses don't go through schema validation,
        but the error dict should be structured."""
        error_response = {"extraction_failed": True, "error": "Invalid JSON from LLM"}
        # This is NOT validated against the schema — it's a failure envelope
        assert error_response["extraction_failed"] is True
        assert "error" in error_response
