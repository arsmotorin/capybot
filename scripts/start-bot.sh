#!/bin/bash

SESSION_NAME="capybot"

if screen -list | grep -q "$SESSION_NAME"; then
    echo "Bot is already running: screen -r $SESSION_NAME"
    exit 1
fi

cd /home/ubuntu/telegram-bot

echo "Building..."
go build -o capybot .

if [ $? -ne 0 ]; then
    echo "Error!"
    exit 1
fi

mkdir -p logs data

screen -dmS "$SESSION_NAME" bash -c "./capybot 2>&1 | tee logs/bot-$(date +%Y%m%d-%H%M%S).log"

sleep 1

if screen -list | grep -q "$SESSION_NAME"; then
    echo "Bot is running. Do you want to connect? Command: screen -r $SESSION_NAME"
else
    echo "Error!"
    exit 1
fi
