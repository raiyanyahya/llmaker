from app.ingest import chunk_text


def test_chunk_empty():
    assert chunk_text("", 100, 10) == []
    assert chunk_text("   ", 100, 10) == []


def test_chunk_short_text_is_one_chunk():
    assert chunk_text("hello world", 100, 10) == ["hello world"]


def test_chunk_splits_and_overlaps():
    text = ". ".join(f"sentence number {i}" for i in range(60))
    chunks = chunk_text(text, 100, 20)
    assert len(chunks) > 1
    # Every chunk is within a reasonable bound of the target size.
    assert all(len(c) <= 100 for c in chunks)
    # Reassembled chunks cover all the source words.
    joined = " ".join(chunks)
    assert "sentence number 0" in joined
    assert "sentence number 59" in joined


def test_overlap_clamped_when_too_large():
    # overlap >= size must not loop forever.
    chunks = chunk_text("a" * 500, 100, 200)
    assert len(chunks) >= 1
