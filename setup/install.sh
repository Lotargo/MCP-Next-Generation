#!/bin/bash

# --- 1. Обеспечение надежности и правильного расположения ---

# Прекращать выполнение скрипта при первой же ошибке.
set -e

# Определяем абсолютный путь к корневой директории проекта (на уровень выше, чем папка 'setup').
# Это позволяет запускать скрипт из любого места в системе.
PROJECT_ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."
echo "Project root detected as: $PROJECT_ROOT"

# Переходим в корневую директорию для выполнения всех последующих команд.
# Это аналог `pushd %PROJECT_ROOT%`
cd "$PROJECT_ROOT"

# --- 2. Установка зависимостей ---

echo
echo "[1/3] Installing Poetry..."
# Используем python3 -m pip для надежности, так как 'pip' может указывать на старый Python 2.
python3 -m pip install poetry

echo
echo "[2/3] Installing Python dependencies..."
# Устанавливаем корневые зависимости
echo "Installing root Python dependencies..."
poetry install

# Устанавливаем зависимости для каждого Python-инструмента
echo "Installing dependencies for individual Python tools..."
for tool_dir in MCP-NG/tools/python/*/; do
    if [ -f "${tool_dir}pyproject.toml" ]; then
        tool_name=$(basename "$tool_dir")
        echo "--- Installing dependencies for $tool_name ---"
        (
            cd "$tool_dir"
            poetry install
            if [ $? -ne 0 ]; then
                echo "ERROR: Failed to install dependencies for $tool_name" >&2
                exit 1
            fi
        )
    fi
done

echo
echo "[3/3] Building Go binaries for Linux/macOS..."
# Вызываем сборочный скрипт по относительному пути.
# Убедитесь, что ваш новый скрипт сборки называется `build.sh`
# и лежит в папке `setup`.
./setup/build.sh

echo
echo "Installation complete."