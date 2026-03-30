# CQ Drive — File Sharing Across Devices

> Transfer files between your devices and teammates — no USB, no email, no cloud storage setup.

---

## 1. Upload

Upload a single file, a folder, or multiple files in one command.

```bash
cq drive upload model.pt                    # single file
cq drive upload ./results/ --as exp-001     # folder
cq drive upload *.csv --to datasets/        # multiple files
```

CQ auto-detects whether the path is a file or folder. Supported flags:

| Flag | Description |
|------|-------------|
| `--to <path>` | Destination prefix in storage |
| `--as <name>` | Rename the file or folder on upload |

---

## 2. Download

Download a file from CQ Drive to your local machine.

```bash
cq drive download models/best.pt
cq drive download models/best.pt -o ./local/
```

| Flag | Description |
|------|-------------|
| `-o <dir>` | Output directory (default: current directory) |

---

## 3. Share

Generate a presigned URL that anyone can download — no account required.

```bash
cq drive share checkpoints/model.pt              # 1hr default
cq drive share checkpoints/model.pt --ttl 24h    # 24 hours
# → https://...supabase.co/storage/v1/object/sign/...?token=xxx
```

- Default TTL: **1 hour**
- Maximum TTL: **7 days**
- The link works for anyone — no CQ account needed.

---

## 4. Dataset

Versioned dataset management with content-hash deduplication.

```bash
cq drive dataset upload ./scan_data --as dental-v3
cq drive dataset list
cq drive dataset list dental-v3          # show versions of a specific dataset
cq drive dataset pull dental-v3          # download latest version
```

CQ uses SHA256 content hashing to deduplicate files across versions — only changed files are uploaded. A `.cqdata` file is written locally after upload to track the dataset version in git.

---

## 5. Dataset Sync

Keep datasets in sync automatically after `git pull`.

```bash
cq dataset sync              # pull changed datasets only
cq dataset sync --dry-run    # preview what would be synced
```

Run `cq init` once in your repo to install a `post-merge` git hook. The workflow:

1. Machine A uploads dataset → `.cqdata` file is updated
2. `git push` the `.cqdata` file
3. Machine B runs `git pull` → hook triggers `cq dataset sync` automatically
4. Only datasets with changed content hashes are downloaded

---

## 6. Resilient Transfer

Large file transfers are resumable and survive network interruptions.

- **TUS resumable upload** for files >= 10MB (6MB chunks)
- **Range-based resume download** with `.part` file persistence — picks up where it left off
- Survives WSL2 NAT drops, flaky Wi-Fi, VPN reconnects

No special flags needed — resilience is automatic based on file size.

---

## 7. MCP Tools

When working inside Claude Code, use MCP tools directly instead of the CLI:

| Tool | Description |
|------|-------------|
| `cq_drive_upload` | Upload a file to CQ Drive |
| `cq_drive_download` | Download a file from CQ Drive |
| `cq_drive_share` | Generate a presigned download URL |
| `cq_drive_list` | List files in storage |
| `cq_drive_dataset_upload` | Upload a directory as a versioned dataset |
| `cq_drive_dataset_pull` | Download a dataset by name |
| `cq_drive_dataset_list` | List datasets and their versions |

---

## 8. Use Cases

**Transfer model between GPU server and laptop**

```bash
# On GPU server
cq drive upload ./checkpoints/epoch_100.pt --to models/

# On laptop
cq drive download models/epoch_100.pt
```

**Share results with a teammate (no account needed)**

```bash
cq drive share results/exp042.json --ttl 24h
# Send the link via Slack, email, or anywhere
```

**Keep datasets synced across machines**

```bash
# One-time setup on each machine
cq init   # installs post-merge git hook

# Upload dataset and commit the .cqdata file
cq drive dataset upload ./data/processed --as dental-v3
git add .cqdata && git commit -m "chore: update dental-v3 dataset ref"
git push

# On another machine — sync happens automatically after git pull
git pull
# hook runs: cq dataset sync
```

**Resume a failed large file transfer**

```bash
# Start upload — interrupted by network drop
cq drive upload ./big_model_10gb.pt

# Re-run the same command — resumes from last chunk
cq drive upload ./big_model_10gb.pt
```
