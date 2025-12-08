#!/bin/bash

SESSION_NAME="capybot"

if ! screen -list | grep -q "$SESSION_NAME"; then
    echo "Bot is not running."
    exit 1
fi

echo "Stopping..."
screen -S "$SESSION_NAME" -X stuff "^C"
sleep 2

if screen -list | grep -q "$SESSION_NAME"; then
    screen -S "$SESSION_NAME" -X quit
fi

pkill -f "capybot" 2>/dev/null

echo "Bot stopped."
