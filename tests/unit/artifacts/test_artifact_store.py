"""Tests for local artifact store."""

import asyncio

import pytest

from c4.artifacts.store import LocalArtifactStore


@pytest.fixture
def store(tmp_path):
    return LocalArtifactStore(base_path=tmp_path / "artifacts")


@pytest.fixture
def sample_file(tmp_path):
    f = tmp_path / "model.pt"
    f.write_bytes(b"fake model weights 12345")
    return f


class TestLocalArtifactStore:
    def test_save_and_get(self, store, sample_file):
        ref = asyncio.get_event_loop().run_until_complete(
            store.save("T-001-0", sample_file, "output")
        )
        assert ref.name == "model.pt"
        assert ref.type == "output"
        assert ref.content_hash != ""
        assert ref.size_bytes > 0
        assert ref.version == 1

        path = asyncio.get_event_loop().run_until_complete(
            store.get("T-001-0", "model.pt")
        )
        assert path.exists()

    def test_content_addressable_dedup(self, store, sample_file):
        ref1 = asyncio.get_event_loop().run_until_complete(
            store.save("T-001-0", sample_file, "output")
        )
        ref2 = asyncio.get_event_loop().run_until_complete(
            store.save("T-002-0", sample_file, "output")
        )
        # Same content → same hash
        assert ref1.content_hash == ref2.content_hash

    def test_versioning(self, store, tmp_path):
        f1 = tmp_path / "metrics.json"
        f1.write_text('{"loss": 0.5}')
        asyncio.get_event_loop().run_until_complete(
            store.save("T-001-0", f1, "output")
        )

        f1.write_text('{"loss": 0.3}')
        ref2 = asyncio.get_event_loop().run_until_complete(
            store.save("T-001-0", f1, "output")
        )
        assert ref2.version == 2

    def test_list_artifacts(self, store, sample_file):
        asyncio.get_event_loop().run_until_complete(
            store.save("T-001-0", sample_file, "output")
        )
        items = asyncio.get_event_loop().run_until_complete(
            store.list("T-001-0")
        )
        assert len(items) == 1
        assert items[0].name == "model.pt"

    def test_delete_artifact(self, store, sample_file):
        asyncio.get_event_loop().run_until_complete(
            store.save("T-001-0", sample_file, "output")
        )
        deleted = asyncio.get_event_loop().run_until_complete(
            store.delete("T-001-0", "model.pt")
        )
        assert deleted is True

        items = asyncio.get_event_loop().run_until_complete(
            store.list("T-001-0")
        )
        assert len(items) == 0

    def test_get_not_found(self, store):
        with pytest.raises(FileNotFoundError):
            asyncio.get_event_loop().run_until_complete(
                store.get("T-999-0", "nonexistent")
            )

    def test_save_nonexistent_file(self, store, tmp_path):
        with pytest.raises(FileNotFoundError):
            asyncio.get_event_loop().run_until_complete(
                store.save("T-001-0", tmp_path / "nope.bin", "output")
            )
