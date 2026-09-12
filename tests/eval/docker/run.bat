@echo off
rem Run godai-eval in the image build.bat makes; all arguments go to godai-eval.
rem The repo is mounted at /work, so tasks are read from and results written to
rem the working tree - but godai and godai-eval run from the image, so rebuild
rem it after changing Go code.
setlocal

for %%I in ("%~dp0..\..\..") do set "REPO_ROOT=%%~fI"

docker run --rm -it --init -e ANTHROPIC_API_KEY -e CLAUDE_CODE_OAUTH_TOKEN -v "%REPO_ROOT%:/work" godai-eval %*
exit /b %errorlevel%
