"""MNIST Mini Classification — sklearn + simple CNN comparison."""
import json
import os
import time
from pathlib import Path

import numpy as np

DATA_DIR = Path(os.environ.get("MNIST_DATA", "data/mnist"))
RESULT_FILE = os.environ.get("C5_RESULT_FILE", "")

def load_data():
    X_train = np.load(DATA_DIR / "X_train.npy").astype(np.float32) / 255.0
    y_train = np.load(DATA_DIR / "y_train.npy")
    X_test = np.load(DATA_DIR / "X_test.npy").astype(np.float32) / 255.0
    y_test = np.load(DATA_DIR / "y_test.npy")
    return X_train, y_train, X_test, y_test

def sklearn_experiment(X_train, y_train, X_test, y_test):
    from sklearn.ensemble import RandomForestClassifier
    from sklearn.linear_model import LogisticRegression
    from sklearn.metrics import accuracy_score, classification_report

    results = {}

    # Logistic Regression
    t0 = time.time()
    lr = LogisticRegression(max_iter=500, solver="lbfgs")
    lr.fit(X_train, y_train)
    lr_time = time.time() - t0
    lr_pred = lr.predict(X_test)
    lr_acc = accuracy_score(y_test, lr_pred)
    results["logistic_regression"] = {"accuracy": round(lr_acc, 4), "train_time_s": round(lr_time, 3)}
    print(f"LogisticRegression: acc={lr_acc:.4f} ({lr_time:.2f}s)")

    # Random Forest
    t0 = time.time()
    rf = RandomForestClassifier(n_estimators=100, random_state=42)
    rf.fit(X_train, y_train)
    rf_time = time.time() - t0
    rf_pred = rf.predict(X_test)
    rf_acc = accuracy_score(y_test, rf_pred)
    results["random_forest"] = {"accuracy": round(rf_acc, 4), "train_time_s": round(rf_time, 3)}
    print(f"RandomForest:       acc={rf_acc:.4f} ({rf_time:.2f}s)")

    # Best model report
    best = max(results, key=lambda k: results[k]["accuracy"])
    print(f"\nBest: {best} (acc={results[best]['accuracy']})")
    print(f"\nClassification Report ({best}):")
    best_pred = lr_pred if best == "logistic_regression" else rf_pred
    print(classification_report(y_test, best_pred, zero_division=0))

    return results

def try_pytorch(X_train, y_train, X_test, y_test):
    """Simple 2-layer MLP if torch is available."""
    try:
        import torch
        import torch.nn as nn
    except ImportError:
        print("PyTorch not available, skipping CNN experiment")
        return None

    device = "cuda" if torch.cuda.is_available() else "cpu"
    print(f"\nPyTorch device: {device}")

    X_tr = torch.tensor(X_train).to(device)
    y_tr = torch.tensor(y_train, dtype=torch.long).to(device)
    X_te = torch.tensor(X_test).to(device)
    y_te = torch.tensor(y_test, dtype=torch.long).to(device)

    model = nn.Sequential(
        nn.Linear(784, 128), nn.ReLU(), nn.Dropout(0.2),
        nn.Linear(128, 64), nn.ReLU(), nn.Dropout(0.2),
        nn.Linear(64, 10),
    ).to(device)

    opt = torch.optim.Adam(model.parameters(), lr=1e-3)
    loss_fn = nn.CrossEntropyLoss()

    t0 = time.time()
    for epoch in range(30):
        model.train()
        opt.zero_grad()
        loss = loss_fn(model(X_tr), y_tr)
        loss.backward()
        opt.step()
        if (epoch + 1) % 10 == 0:
            model.eval()
            with torch.no_grad():
                acc = (model(X_te).argmax(1) == y_te).float().mean().item()
            print(f"  epoch {epoch+1:3d}: loss={loss.item():.4f}  test_acc={acc:.4f}")

    train_time = time.time() - t0
    model.eval()
    with torch.no_grad():
        final_acc = (model(X_te).argmax(1) == y_te).float().mean().item()

    print(f"MLP:                acc={final_acc:.4f} ({train_time:.2f}s, {device})")
    return {"accuracy": round(final_acc, 4), "train_time_s": round(train_time, 3), "device": device}

def main():
    print("=" * 50)
    print("MNIST Mini Classification Experiment")
    print("=" * 50)

    X_train, y_train, X_test, y_test = load_data()
    print(f"Data: train={X_train.shape}, test={X_test.shape}")
    print(f"Classes: {sorted(set(y_train))}\n")

    results = sklearn_experiment(X_train, y_train, X_test, y_test)

    mlp_result = try_pytorch(X_train, y_train, X_test, y_test)
    if mlp_result:
        results["mlp_pytorch"] = mlp_result

    # Summary
    print("\n" + "=" * 50)
    print("SUMMARY")
    print("=" * 50)
    best_name = max(results, key=lambda k: results[k]["accuracy"])
    for name, r in sorted(results.items(), key=lambda x: -x[1]["accuracy"]):
        marker = " ★" if name == best_name else ""
        print(f"  {name:25s} acc={r['accuracy']:.4f}  ({r['train_time_s']:.2f}s){marker}")

    # Write result file for C5
    output = {"status": "ok", "best_model": best_name, "results": results}
    if RESULT_FILE:
        Path(RESULT_FILE).write_text(json.dumps(output))
    print(f"\nresult={json.dumps(output)}")

if __name__ == "__main__":
    main()
