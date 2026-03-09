#!/usr/bin/env python3
"""MNIST 학습 예시 — GPU Worker E2E 검증용"""

import time
import torch
import torch.nn as nn
import torch.optim as optim
from torchvision import datasets, transforms
from torch.utils.data import DataLoader

print(f"PyTorch {torch.__version__}, CUDA: {torch.cuda.is_available()}")
device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
print(f"Device: {device}")
if torch.cuda.is_available():
    print(f"GPU: {torch.cuda.get_device_name()}")

transform = transforms.Compose([
    transforms.ToTensor(),
    transforms.Normalize((0.1307,), (0.3081,)),
])
train_data = datasets.MNIST("/tmp/mnist", train=True, download=True, transform=transform)
test_data = datasets.MNIST("/tmp/mnist", train=False, transform=transform)
train_loader = DataLoader(train_data, batch_size=256, shuffle=True, num_workers=2)
test_loader = DataLoader(test_data, batch_size=1000)

model = nn.Sequential(
    nn.Flatten(),
    nn.Linear(784, 256), nn.ReLU(), nn.Dropout(0.2),
    nn.Linear(256, 128), nn.ReLU(), nn.Dropout(0.2),
    nn.Linear(128, 10),
).to(device)

optimizer = optim.Adam(model.parameters(), lr=1e-3)
loss_fn = nn.CrossEntropyLoss()

start = time.time()
for epoch in range(3):
    model.train()
    total_loss = 0
    for X, y in train_loader:
        X, y = X.to(device), y.to(device)
        loss = loss_fn(model(X), y)
        optimizer.zero_grad()
        loss.backward()
        optimizer.step()
        total_loss += loss.item()

    model.eval()
    correct = 0
    with torch.no_grad():
        for X, y in test_loader:
            X, y = X.to(device), y.to(device)
            correct += (model(X).argmax(1) == y).sum().item()
    acc = correct / len(test_data) * 100
    print(f"Epoch {epoch+1}/3  loss={total_loss/len(train_loader):.4f}  acc={acc:.1f}%")

elapsed = time.time() - start
print(f"Done in {elapsed:.1f}s")
