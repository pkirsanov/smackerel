"""Unit tests for receipt detection heuristics."""

from app.receipt_detection import detect_receipt_content


class TestH001BillContentType:
    """H-001: content_type == 'bill' always triggers."""

    def test_bill_type_with_empty_text(self):
        assert detect_receipt_content("", content_type="bill") is True

    def test_bill_type_with_any_text(self):
        assert detect_receipt_content("random text", content_type="bill") is True


class TestH002AmountPlusBillingKeyword:
    """H-002: Amount pattern + billing keyword."""

    def test_dollar_amount_and_receipt_keyword(self):
        text = "Your receipt from Square. Total: $4.75"
        assert detect_receipt_content(text) is True

    def test_euro_amount_and_payment_keyword(self):
        text = "Payment confirmed: €47,50 charged to your card"
        assert detect_receipt_content(text) is True

    def test_usd_suffix_and_invoice_keyword(self):
        text = "Invoice #1234. Amount: 99.99 USD"
        assert detect_receipt_content(text) is True

    def test_amount_without_keyword(self):
        text = "The temperature today is $5 million degrees"
        # "$5" matches amount pattern but no billing keywords
        assert detect_receipt_content(text) is False

    def test_keyword_without_amount(self):
        text = "Your subscription has been updated. No charge."
        # "subscription" is a keyword but there's no amount pattern
        assert detect_receipt_content(text) is False


class TestH003ExplicitReceiptLanguage:
    """H-003: 'receipt' or 'invoice' in first 500 chars + amount."""

    def test_receipt_in_first_500_with_amount(self):
        text = "Here is your receipt\n" + "x" * 100 + "\nTotal: $108.25"
        assert detect_receipt_content(text) is True

    def test_invoice_in_subject_with_amount(self):
        text = "Monthly hosting charges $48.00"
        assert detect_receipt_content(text, subject="DigitalOcean Invoice") is True

    def test_receipt_word_far_in_text(self):
        # 'receipt' is past char 500, so H-003 title check should fail
        # but H-002 would still match (amount + "total" keyword)
        text = "x" * 600 + " receipt Total: $10.00"
        assert detect_receipt_content(text) is True  # H-002 fires


class TestH004TelegramOCR:
    """H-004: Telegram OCR captures are always receipt-likely."""

    def test_telegram_media_source(self):
        assert detect_receipt_content(
            "some OCR text", content_type="media", source_id="telegram"
        ) is True

    def test_telegram_ocr_type(self):
        assert detect_receipt_content(
            "blurry text", content_type="ocr", source_id="telegram"
        ) is True

    def test_telegram_text_message_not_receipt(self):
        # Regular telegram text is NOT auto-receipted
        assert detect_receipt_content(
            "hello world", content_type="text", source_id="telegram"
        ) is False


class TestH005EmailSenderPatterns:
    """H-005: Known receipt sender patterns."""

    def test_receipts_at_sender(self):
        assert detect_receipt_content(
            "Your order", sender="receipts@squareup.com"
        ) is True

    def test_billing_at_sender(self):
        assert detect_receipt_content(
            "Monthly charge", sender="billing@netflix.com"
        ) is True

    def test_noreply_requires_keyword(self):
        # noreply@ alone is not enough — need receipt keyword in subject
        assert detect_receipt_content(
            "Your account was updated",
            sender="noreply@example.com",
            subject="Account settings",
        ) is False

    def test_noreply_with_receipt_subject(self):
        assert detect_receipt_content(
            "Thank you for your order",
            sender="noreply@amazon.com",
            subject="Your receipt from Amazon",
        ) is True


class TestAdversarialCases:
    """Non-receipt content that must NOT trigger detection (BS-020)."""

    def test_marketing_newsletter(self):
        """Amazon marketing email with no amount, no order number."""
        text = (
            "Check out these amazing deals! New arrivals this week from "
            "your favorite brands. Shop now at Amazon.com for great savings."
        )
        assert detect_receipt_content(text, source_id="gmail") is False

    def test_news_article_with_dollar_mention(self):
        text = "The company raised $50 million in Series B funding."
        assert detect_receipt_content(text) is False

    def test_empty_text(self):
        assert detect_receipt_content("") is False

    def test_none_like_text(self):
        assert detect_receipt_content("", content_type="note") is False


class TestChaoPathologicalInput:
    """CHAOS: Pathological input that must not crash or hang."""

    def test_all_emoji_input(self):
        """All-emoji text should not trigger receipt detection."""
        text = "🏪" * 500 + "💰" * 500
        assert detect_receipt_content(text) is False

    def test_binary_data(self):
        """Binary-like content should not crash or trigger."""
        text = "\x00\x01\x02\xff\xfe\xfd" * 100
        assert detect_receipt_content(text) is False

    def test_empty_string(self):
        assert detect_receipt_content("") is False

    def test_whitespace_only(self):
        assert detect_receipt_content("   \n\t\r\n  ") is False

    def test_extremely_long_input(self):
        """100KB+ input must not hang — should be capped internally."""
        text = "A" * 200_000 + " receipt $10.00 total"
        # The receipt keyword + amount is past the 100K cap, so H-002
        # won't see it. But the first 100K has no receipt patterns.
        result = detect_receipt_content(text)
        assert result is False

    def test_long_input_with_receipt_in_front(self):
        """Receipt pattern at start of long input should still fire."""
        text = "Your receipt total: $47.50 " + "X" * 200_000
        assert detect_receipt_content(text) is True

    def test_null_bytes_in_text(self):
        """Null bytes should not crash regex."""
        text = "receipt\x00total\x00$15.00"
        assert detect_receipt_content(text) is True

    def test_unicode_currency_symbols(self):
        """Various currency symbols must work."""
        assert detect_receipt_content("Payment: €47,50 charged") is True
        assert detect_receipt_content("Invoice total: £89.99") is True

    def test_repeated_formula_chars(self):
        """Input that looks like CSV injection should not crash."""
        text = "=" * 10000
        assert detect_receipt_content(text) is False
