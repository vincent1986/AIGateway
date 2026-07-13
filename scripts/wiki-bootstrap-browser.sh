#!/usr/bin/env bash
set -euo pipefail
export PATH="/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin:/opt/homebrew/bin"

defaults write com.google.Chrome AllowJavascriptFromAppleEvents -bool true 2>/dev/null || true
defaults write com.apple.Safari AllowJavaScriptFromAppleEvents -bool true 2>/dev/null || true

# Probe Safari JS
osascript <<'EOF'
tell application "Safari"
  activate
  if (count of windows) = 0 then
    make new document with properties {URL:"https://github.com/vincent1986/AIGateway/wiki/_new"}
  else
    set URL of current tab of window 1 to "https://github.com/vincent1986/AIGateway/wiki/_new"
  end if
  delay 5
  try
    set r to do JavaScript "document.title + ' | ' + location.href" in current tab of window 1
    return "SAFARI_OK: " & r
  on error errMsg
    return "SAFARI_ERR: " & errMsg
  end try
end tell
EOF
