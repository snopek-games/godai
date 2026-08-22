@echo off
rem Build the godai-eval image; extra arguments are passed to `docker build`.
setlocal

set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "REPO_ROOT=%%~fI"

docker build --file "%SCRIPT_DIR%Dockerfile" --tag godai-eval %* "%REPO_ROOT%"
exit /b %errorlevel%
