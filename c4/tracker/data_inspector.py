"""Data profiling - inspect input data shapes, types, and statistics.

Absorbed from piq/piqr/data_inspector.py.
Supports numpy, pandas, and torch tensors (all optional imports).
"""

from __future__ import annotations

import hashlib
import logging
from typing import Any

logger = logging.getLogger(__name__)


def inspect_data(data: Any, name: str = "data") -> dict:
    """Profile input data for experiment tracking.

    Args:
        data: Input data (numpy array, pandas DataFrame, torch tensor, or basic type)
        name: Variable name for the profile

    Returns:
        Dict with shape, dtype, stats, hash
    """
    profile: dict[str, Any] = {"name": name, "type": type(data).__name__}

    try:
        # numpy array
        if _is_numpy(data):
            return _profile_numpy(data, profile)

        # pandas DataFrame
        if _is_pandas_df(data):
            return _profile_pandas(data, profile)

        # torch Tensor
        if _is_torch_tensor(data):
            return _profile_torch(data, profile)

        # Basic types
        if isinstance(data, (list, tuple)):
            profile["length"] = len(data)
        elif isinstance(data, dict):
            profile["keys"] = list(data.keys())[:20]
        elif isinstance(data, str):
            profile["length"] = len(data)

    except Exception as e:
        profile["error"] = str(e)

    return profile


def compute_data_hash(data: Any) -> str:
    """Compute a content hash for data versioning."""
    try:
        if _is_numpy(data):
            return hashlib.sha256(data.tobytes()).hexdigest()[:16]
        elif _is_torch_tensor(data):
            return hashlib.sha256(
                data.cpu().detach().numpy().tobytes()
            ).hexdigest()[:16]
        elif _is_pandas_df(data):
            import pandas as pd

            return hashlib.sha256(
                pd.util.hash_pandas_object(data).values.tobytes()
            ).hexdigest()[:16]
        else:
            return hashlib.sha256(str(data).encode()).hexdigest()[:16]
    except Exception:
        return "unknown"


def _is_numpy(obj: Any) -> bool:
    try:
        import numpy as np
        return isinstance(obj, np.ndarray)
    except ImportError:
        return False


def _is_pandas_df(obj: Any) -> bool:
    try:
        import pandas as pd
        return isinstance(obj, pd.DataFrame)
    except ImportError:
        return False


def _is_torch_tensor(obj: Any) -> bool:
    try:
        import torch
        return isinstance(obj, torch.Tensor)
    except ImportError:
        return False


def _profile_numpy(data: Any, profile: dict) -> dict:
    import numpy as np

    profile["shape"] = list(data.shape)
    profile["dtype"] = str(data.dtype)
    profile["hash"] = compute_data_hash(data)
    if np.issubdtype(data.dtype, np.number) and data.size > 0:
        profile["stats"] = {
            "mean": float(np.nanmean(data)),
            "std": float(np.nanstd(data)),
            "min": float(np.nanmin(data)),
            "max": float(np.nanmax(data)),
        }
        profile["missing"] = int(np.isnan(data).sum()) if np.issubdtype(data.dtype, np.floating) else 0
    return profile


def _profile_pandas(data: Any, profile: dict) -> dict:
    profile["shape"] = list(data.shape)
    profile["columns"] = list(data.columns)[:50]
    profile["dtypes"] = {str(k): str(v) for k, v in data.dtypes.items()}
    profile["hash"] = compute_data_hash(data)
    profile["missing"] = int(data.isnull().sum().sum())
    return profile


def _profile_torch(data: Any, profile: dict) -> dict:
    profile["shape"] = list(data.shape)
    profile["dtype"] = str(data.dtype)
    profile["device"] = str(data.device)
    profile["requires_grad"] = data.requires_grad
    if data.numel() > 0 and data.is_floating_point():
        profile["stats"] = {
            "mean": float(data.mean().item()),
            "std": float(data.std().item()),
            "min": float(data.min().item()),
            "max": float(data.max().item()),
        }
    return profile
