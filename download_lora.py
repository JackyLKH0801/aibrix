from huggingface_hub import snapshot_download
import os
import sys

model_id = "bharati2324/Qwen2.5-1.5B-Instruct-Code-LoRA-r16v2"
local_dir = "./lora-adapter-real"

print(f"Downloading {model_id} to {local_dir}...")
try:
    snapshot_download(repo_id=model_id, local_dir=local_dir)
    print("Download complete.")
except Exception as e:
    print(f"Error downloading: {e}")
    sys.exit(1)
