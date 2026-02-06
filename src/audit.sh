#!/bin/bash
# ==============================================================================
# ABox Audit Tool
# ==============================================================================

run_audit() {
    local target_dir="${1:-.}"
    target_dir=$(cd "$target_dir" && pwd)
    
    echo "--- ABox Security Audit: $target_dir ---"
    echo
    
    # 1. Check for sensitive files
    echo "[1] Checking for sensitive files..."
    local secrets_found=0
    for secret in .env .ssh .aws .gnupg .git/credentials; do
        if [[ -e "$target_dir/$secret" ]]; then
            echo "  [!] WARNING: Found sensitive file/directory: $secret"
            secrets_found=$((secrets_found + 1))
        fi
    done
    
    if find "$target_dir" -maxdepth 2 -name "*key" -o -name "*.pem" | grep -q .; then
        echo "  [!] WARNING: Found potential private keys (*key, *.pem)"
        secrets_found=$((secrets_found + 1))
    fi
    
    if [[ $secrets_found -eq 0 ]]; then
        echo "  [✓] No common secrets found."
    fi
    echo
    
    # 2. Check for .abxignore
    echo "[2] Checking for exclusion configuration..."
    if [[ -f "$target_dir/.abxignore" ]]; then
        echo "  [✓] .abxignore found."
    else
        echo "  [!] NOTICE: .abxignore not found. Using automatic exclusions only."
    fi
    echo
    
    # 3. Check runtime configuration
    echo "[3] Checking runtime environment..."
    local runtime=$(detect_runtime 2>/dev/null)
    if [[ -n "$runtime" ]]; then
        echo "  [✓] Container runtime: $runtime"
        if [[ "$runtime" == "docker" ]]; then
            if groups | grep -q "docker"; then
                echo "  [✓] User is in docker group (non-root execution)."
            else
                echo "  [!] NOTICE: User not in docker group; may require sudo."
            fi
        fi
    else
        echo "  [!] ERROR: No container runtime detected."
    fi
    echo
    
    # 4. Check for insecure mount patterns (simple check)
    echo "[4] Checking for insecure workspace patterns..."
    if [[ "$target_dir" == "$HOME" ]]; then
        echo "  [!] CRITICAL: Running ABox in HOME directory is highly discouraged."
    elif [[ "$target_dir" == "/" ]]; then
        echo "  [!] CRITICAL: Running ABox in ROOT directory is highly discouraged."
    else
        echo "  [✓] Workspace directory seems appropriate."
    fi
    echo
    
    echo "Audit complete."
}
