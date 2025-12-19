#!/bin/bash

# --- 1. Обеспечение надежности и правильного расположения ---

# Прекращать выполнение при первой же ошибке. Аналог "if errorlevel neq 0 exit".
set -e

# Переходим в корневую директорию проекта, где бы скрипт ни был запущен.
# Это аналог `pushd %~dp0..`
PROJECT_ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."
cd "$PROJECT_ROOT"

echo "Project root detected as: $(pwd)"

# ========================= НАЧАЛО БЛОКА АВТО-КОНФИГУРАЦИИ GO.WORK =========================
echo
echo "[A] Re-initializing Go workspace to match current project structure..."

# Удаляем старые файлы, чтобы избежать конфликтов. `-f` игнорирует ошибку, если файлов нет.
rm -f go.work go.work.sum

# Создаем новый пустой go.work. Если команда провалится, `set -e` остановит скрипт.
go work init

# Ищем ВСЕ файлы go.mod рекурсивно и добавляем их родительские папки в go.work
echo "[B] Automatically discovering and adding Go modules..."
# `find` ищет файлы, `while read` безопасно обрабатывает пути с пробелами
find . -name "go.mod" | while read -r modfile; do
    # `dirname` получает путь к папке из пути к файлу
    module_dir=$(dirname "$modfile")
    echo "     - Found module, adding: $module_dir"
    go work use "$module_dir"
done
echo
# ========================== КОНЕЦ БЛОКА АВТО-КОНФИГУРАЦИИ ===========================


# --- 2. Сборка бинарных файлов ---

echo "Building Go binaries for Linux..."

# 3. Устанавливаем переменные окружения для Go.
# Для macOS измените GOOS на "darwin".
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0

# 4. Создаем папку bin. `-p` создает родительские папки и не ругается, если она уже есть.
BIN_DIR="./MCP-NG/bin"
mkdir -p "$BIN_DIR"

# 5. Компилируем Meta-Server. Теперь go.work всегда будет актуальным.
echo "Building Meta-Server..."
go build -o "$BIN_DIR/server" ./MCP-NG/server/cmd/server

# 6. Компилируем все существующие Go утилиты
echo "Building Go tools..."
for d in ./MCP-NG/tools/go/*; do
  # Проверяем, что это действительно директория
  if [ -d "$d" ]; then
    tool_name=$(basename "$d")
    echo "Building $tool_name..."
    go build -o "$BIN_DIR/$tool_name" "./$d"
  fi
done

# 7. Установка зависимостей для Python инструментов
echo "Installing dependencies for Python tools..."
for d in ./MCP-NG/tools/python/*; do
  if [ -d "$d" ] && [ -f "$d/pyproject.toml" ]; then
    tool_name=$(basename "$d")
    echo "Installing dependencies for $tool_name..."
    (cd "$d" && poetry install)
  fi
done

echo
echo "Build complete. Binaries are in $BIN_DIR"
