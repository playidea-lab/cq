#!/bin/bash
# PI Lab Session Start Hook
# Prints C4 status hint on session startup/resume.

if [[ -d ".c4" ]]; then
    echo "C4 프로젝트: /c4-status로 상태 확인, /c4-quick으로 빠른 시작"
fi
