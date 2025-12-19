@echo off
setlocal

:: Определяем корневую директорию проекта. %~dp0 - это путь к текущему скрипту (папка setup).
:: Мы поднимаемся на один уровень вверх (..), чтобы попасть в корень проекта.
set "PROJECT_ROOT=%~dp0.."
echo Project root detected as: %PROJECT_ROOT%

echo.
echo [1/3] Installing Poetry...
:: Используем более надежный способ вызова pip, который не зависит от переменной PATH
python -m pip install poetry
if %errorlevel% neq 0 (
    echo Failed to install Poetry.
    exit /b 1
)

echo.
echo [2/3] Installing Python dependencies...
:: Временно переходим в корень проекта, чтобы poetry нашел pyproject.toml
pushd "%PROJECT_ROOT%"
poetry install
if %errorlevel% neq 0 (
    echo Failed to install Python dependencies.
    popd
    exit /b 1
)
:: Возвращаемся обратно
popd

echo.
echo [3/3] Building Go binaries for Windows...
:: Вызываем скрипт сборки.
call "%PROJECT_ROOT%\setup\build_windows.bat"
if %errorlevel% neq 0 (
    echo Failed to build Go binaries.
    exit /b 1
)

echo.
echo Installation complete.
endlocal