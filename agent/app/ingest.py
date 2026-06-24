"""Document ingestion helpers: extract text from uploads and split it into
overlapping chunks suitable for embedding."""

from __future__ import annotations

import io


def extract_text(filename: str, data: bytes) -> str:
    """Pull plain text out of an upload. PDFs are parsed; everything else is
    treated as UTF-8 text (best-effort)."""
    if filename.lower().endswith(".pdf"):
        from pypdf import PdfReader

        reader = PdfReader(io.BytesIO(data))
        return "\n\n".join((page.extract_text() or "") for page in reader.pages)
    return data.decode("utf-8", errors="replace")


def chunk_text(text: str, size: int, overlap: int) -> list[str]:
    """Split text into ~size-character chunks with overlap, preferring to break
    on paragraph/whitespace boundaries so chunks stay readable."""
    text = text.strip()
    if not text:
        return []
    if overlap >= size:
        overlap = size // 4

    chunks: list[str] = []
    start = 0
    n = len(text)
    while start < n:
        end = min(start + size, n)
        if end < n:
            # Back off to the nearest paragraph/space boundary for a cleaner cut.
            window = text[start:end]
            for sep in ("\n\n", "\n", ". ", " "):
                idx = window.rfind(sep)
                if idx > size // 2:
                    end = start + idx + len(sep)
                    break
        chunk = text[start:end].strip()
        if chunk:
            chunks.append(chunk)
        if end >= n:
            break
        start = max(end - overlap, start + 1)
    return chunks
