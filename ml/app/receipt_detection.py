"""Receipt detection heuristics for the ML sidecar.

Pre-LLM heuristic filter that determines whether content is likely a
receipt or invoice. Avoids expensive LLM calls on non-receipt content.
"""

import re

# Amount patterns — ported from Go subscriptions.go patterns and extended
AMOUNT_PATTERN = re.compile(
    r"(?:"
    r"\$\s*\d+\.?\d*"
    r"|\d+\.?\d*\s*(?:USD|EUR|GBP|CAD|AUD)"
    r"|(?:USD|EUR|GBP|CAD|AUD)\s*\d+\.?\d*"
    r"|€\s*\d+[.,]?\d*"
    r"|£\s*\d+\.?\d*"
    r")",
    re.IGNORECASE,
)

BILLING_KEYWORDS = [
    "charge",
    "receipt",
    "billing",
    "subscription",
    "monthly",
    "annual",
    "renewal",
    "payment",
    "invoice",
    "order",
    "total",
    "subtotal",
    "tax",
    "tip",
    "amount due",
    "transaction",
    "purchase",
]

# Email sender patterns that indicate receipt emails (H-005)
RECEIPT_SENDER_PATTERNS = [
    "receipts@",
    "billing@",
    "invoice@",
    "noreply@",
    "no-reply@",
    "payments@",
    "orders@",
]


def detect_receipt_content(
    text: str,
    content_type: str = "",
    source_id: str = "",
    sender: str = "",
    subject: str = "",
) -> bool:
    """Determine whether content is likely a receipt or invoice.

    Returns True if any heuristic rule fires, meaning the content
    should be routed through the receipt-extraction-v1 prompt contract.

    Rules (all case-insensitive):
      H-001: content_type is 'bill'
      H-002: amount_pattern matches AND at least one billing_keyword present
      H-003: 'receipt' or 'invoice' in title/first 500 chars AND amount_pattern
      H-004: source is 'telegram' AND content came from OCR (image capture)
      H-005: Email sender domain matches known receipt sender patterns
    """
    if not text and content_type != "bill":
        return False

    text_lower = text.lower() if text else ""

    # H-001: Already classified as bill
    if content_type == "bill":
        return True

    # H-002: Amount pattern + billing keyword
    has_amount = bool(AMOUNT_PATTERN.search(text))
    has_billing_keyword = any(kw in text_lower for kw in BILLING_KEYWORDS)
    if has_amount and has_billing_keyword:
        return True

    # H-003: Explicit receipt/invoice language + amount in first 500 chars
    first_500 = text_lower[:500]
    subject_lower = subject.lower() if subject else ""
    has_receipt_word = "receipt" in first_500 or "invoice" in first_500
    has_receipt_in_subject = "receipt" in subject_lower or "invoice" in subject_lower
    if (has_receipt_word or has_receipt_in_subject) and has_amount:
        return True

    # H-004: Telegram OCR capture — user intent is photographing receipts
    if source_id == "telegram" and content_type in ("media", "image", "ocr"):
        return True

    # H-005: Known receipt sender patterns in email
    if sender:
        sender_lower = sender.lower()
        for pattern in RECEIPT_SENDER_PATTERNS:
            if pattern in sender_lower:
                # Also require receipt keywords in subject for noreply/no-reply
                if pattern in ("noreply@", "no-reply@"):
                    if has_receipt_in_subject or has_billing_keyword:
                        return True
                else:
                    return True

    return False
