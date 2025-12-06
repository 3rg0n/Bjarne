import * as vscode from 'vscode';
import * as cp from 'child_process';
import * as path from 'path';

// Output channel for bjarne logs
let outputChannel: vscode.OutputChannel;

// Diagnostics collection for validation errors
let diagnosticCollection: vscode.DiagnosticCollection;

export function activate(context: vscode.ExtensionContext) {
    console.log('Bjarne extension activated');

    // Create output channel
    outputChannel = vscode.window.createOutputChannel('Bjarne');
    diagnosticCollection = vscode.languages.createDiagnosticCollection('bjarne');

    // Register commands
    context.subscriptions.push(
        vscode.commands.registerCommand('bjarne.generate', generateCode),
        vscode.commands.registerCommand('bjarne.validate', validateCurrentFile),
        vscode.commands.registerCommand('bjarne.chat', openChatPanel),
        outputChannel,
        diagnosticCollection
    );
}

export function deactivate() {
    // Cleanup
}

/**
 * Generate code from a prompt using bjarne
 */
async function generateCode() {
    const prompt = await vscode.window.showInputBox({
        placeHolder: 'What would you have me create?',
        prompt: 'Enter your C/C++ code generation prompt',
        ignoreFocusOut: true
    });

    if (!prompt) {
        return;
    }

    outputChannel.show(true);
    outputChannel.appendLine(`\n=== Bjarne: Generating code ===`);
    outputChannel.appendLine(`Prompt: ${prompt}`);
    outputChannel.appendLine('');

    const bjarnePath = getBjarnePath();

    // Run bjarne in non-interactive mode (pipe prompt via stdin)
    // For now, we'll use the CLI - future: implement as library
    vscode.window.withProgress({
        location: vscode.ProgressLocation.Notification,
        title: 'Bjarne: Generating code...',
        cancellable: true
    }, async (progress, token) => {
        return new Promise<void>((resolve, reject) => {
            const proc = cp.spawn(bjarnePath, ['--non-interactive'], {
                cwd: vscode.workspace.workspaceFolders?.[0]?.uri.fsPath,
                env: { ...process.env }
            });

            let stdout = '';
            let stderr = '';

            proc.stdout.on('data', (data) => {
                stdout += data.toString();
                outputChannel.append(data.toString());
            });

            proc.stderr.on('data', (data) => {
                stderr += data.toString();
                outputChannel.append(data.toString());
            });

            // Send prompt to stdin
            proc.stdin.write(prompt + '\n');
            proc.stdin.end();

            token.onCancellationRequested(() => {
                proc.kill();
                reject(new Error('Cancelled'));
            });

            proc.on('close', (code) => {
                if (code === 0) {
                    outputChannel.appendLine('\n=== Generation complete ===');
                    // Extract code from output and offer to insert/create file
                    const codeMatch = stdout.match(/```cpp\n([\s\S]*?)\n```/);
                    if (codeMatch) {
                        offerToInsertCode(codeMatch[1]);
                    }
                    resolve();
                } else {
                    outputChannel.appendLine(`\n=== Generation failed (exit code ${code}) ===`);
                    reject(new Error(`bjarne exited with code ${code}`));
                }
            });
        });
    });
}

/**
 * Validate the current C/C++ file using bjarne
 */
async function validateCurrentFile() {
    const editor = vscode.window.activeTextEditor;
    if (!editor) {
        vscode.window.showWarningMessage('No active editor');
        return;
    }

    const document = editor.document;
    if (!['cpp', 'c'].includes(document.languageId)) {
        vscode.window.showWarningMessage('Not a C/C++ file');
        return;
    }

    outputChannel.show(true);
    outputChannel.appendLine(`\n=== Bjarne: Validating ${document.fileName} ===`);

    // Clear previous diagnostics for this file
    diagnosticCollection.delete(document.uri);

    const bjarnePath = getBjarnePath();
    const filePath = document.uri.fsPath;

    vscode.window.withProgress({
        location: vscode.ProgressLocation.Notification,
        title: 'Bjarne: Validating...',
        cancellable: true
    }, async (progress, token) => {
        return new Promise<void>((resolve, reject) => {
            // Use --validate flag to validate existing file
            const proc = cp.spawn(bjarnePath, ['--validate', filePath], {
                cwd: path.dirname(filePath),
                env: { ...process.env }
            });

            let stdout = '';
            let stderr = '';

            proc.stdout.on('data', (data) => {
                stdout += data.toString();
                outputChannel.append(data.toString());
            });

            proc.stderr.on('data', (data) => {
                stderr += data.toString();
                outputChannel.append(data.toString());
            });

            token.onCancellationRequested(() => {
                proc.kill();
                reject(new Error('Cancelled'));
            });

            proc.on('close', (code) => {
                // Parse diagnostics from output
                const diagnostics = parseDiagnostics(stdout + stderr, document);
                diagnosticCollection.set(document.uri, diagnostics);

                if (code === 0 && diagnostics.length === 0) {
                    outputChannel.appendLine('\n=== All validation gates passed! ===');
                    vscode.window.showInformationMessage('Bjarne: All validation gates passed!');
                } else if (diagnostics.length > 0) {
                    outputChannel.appendLine(`\n=== Found ${diagnostics.length} issue(s) ===`);
                    vscode.window.showWarningMessage(`Bjarne: Found ${diagnostics.length} issue(s)`);
                }
                resolve();
            });
        });
    });
}

/**
 * Open the bjarne chat panel (webview)
 */
async function openChatPanel() {
    // Create webview panel for interactive chat
    const panel = vscode.window.createWebviewPanel(
        'bjarneChat',
        'Bjarne Chat',
        vscode.ViewColumn.Beside,
        {
            enableScripts: true,
            retainContextWhenHidden: true
        }
    );

    panel.webview.html = getChatWebviewContent();

    // Handle messages from webview
    panel.webview.onDidReceiveMessage(async (message) => {
        switch (message.command) {
            case 'send':
                // Handle chat message
                const response = await sendToBjarne(message.text);
                panel.webview.postMessage({ command: 'response', text: response });
                break;
        }
    });
}

/**
 * Get the path to the bjarne binary
 */
function getBjarnePath(): string {
    const config = vscode.workspace.getConfiguration('bjarne');
    const customPath = config.get<string>('binaryPath');

    if (customPath && customPath.length > 0) {
        return customPath;
    }

    // Default: assume bjarne is in PATH
    return process.platform === 'win32' ? 'bjarne.exe' : 'bjarne';
}

/**
 * Parse diagnostic messages from bjarne output
 */
function parseDiagnostics(output: string, document: vscode.TextDocument): vscode.Diagnostic[] {
    const diagnostics: vscode.Diagnostic[] = [];

    // Match clang-tidy style: file:line:col: severity: message
    const regex = /([^:\s]+):(\d+):(\d+):\s*(error|warning|note):\s*(.+)/g;
    let match;

    while ((match = regex.exec(output)) !== null) {
        const line = parseInt(match[2]) - 1;  // 0-indexed
        const col = parseInt(match[3]) - 1;
        const severity = match[4];
        const message = match[5];

        const range = new vscode.Range(line, col, line, col + 1);
        const diagnostic = new vscode.Diagnostic(
            range,
            message,
            severity === 'error' ? vscode.DiagnosticSeverity.Error :
            severity === 'warning' ? vscode.DiagnosticSeverity.Warning :
            vscode.DiagnosticSeverity.Information
        );
        diagnostic.source = 'bjarne';
        diagnostics.push(diagnostic);
    }

    // Match sanitizer errors: at /path:line
    const sanitizerRegex = /(?:ASAN|UBSAN|TSAN|MSan)[^\n]*\n.*?at\s+([^:]+):(\d+)/g;
    while ((match = sanitizerRegex.exec(output)) !== null) {
        const line = parseInt(match[2]) - 1;
        const range = new vscode.Range(line, 0, line, 100);
        const diagnostic = new vscode.Diagnostic(
            range,
            'Sanitizer detected issue (see Output for details)',
            vscode.DiagnosticSeverity.Error
        );
        diagnostic.source = 'bjarne';
        diagnostics.push(diagnostic);
    }

    return diagnostics;
}

/**
 * Offer to insert generated code into editor or new file
 */
async function offerToInsertCode(code: string) {
    const choice = await vscode.window.showQuickPick([
        'Insert at cursor',
        'Create new file',
        'Copy to clipboard'
    ], {
        placeHolder: 'What would you like to do with the generated code?'
    });

    switch (choice) {
        case 'Insert at cursor':
            const editor = vscode.window.activeTextEditor;
            if (editor) {
                editor.edit(editBuilder => {
                    editBuilder.insert(editor.selection.active, code);
                });
            }
            break;
        case 'Create new file':
            const doc = await vscode.workspace.openTextDocument({
                content: code,
                language: 'cpp'
            });
            vscode.window.showTextDocument(doc);
            break;
        case 'Copy to clipboard':
            vscode.env.clipboard.writeText(code);
            vscode.window.showInformationMessage('Code copied to clipboard');
            break;
    }
}

/**
 * Send a message to bjarne and get response
 */
async function sendToBjarne(text: string): Promise<string> {
    // Placeholder - would spawn bjarne process
    return `[Bjarne response to: ${text}]`;
}

/**
 * Get HTML content for the chat webview
 */
function getChatWebviewContent(): string {
    return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Bjarne Chat</title>
    <style>
        body {
            font-family: var(--vscode-font-family);
            padding: 10px;
            color: var(--vscode-foreground);
            background: var(--vscode-editor-background);
        }
        #chat {
            height: calc(100vh - 100px);
            overflow-y: auto;
            border: 1px solid var(--vscode-panel-border);
            padding: 10px;
            margin-bottom: 10px;
        }
        .message {
            margin: 8px 0;
            padding: 8px;
            border-radius: 4px;
        }
        .user {
            background: var(--vscode-input-background);
            text-align: right;
        }
        .assistant {
            background: var(--vscode-editor-inactiveSelectionBackground);
        }
        #input-area {
            display: flex;
            gap: 8px;
        }
        #prompt {
            flex: 1;
            padding: 8px;
            background: var(--vscode-input-background);
            color: var(--vscode-input-foreground);
            border: 1px solid var(--vscode-input-border);
        }
        button {
            padding: 8px 16px;
            background: var(--vscode-button-background);
            color: var(--vscode-button-foreground);
            border: none;
            cursor: pointer;
        }
        button:hover {
            background: var(--vscode-button-hoverBackground);
        }
    </style>
</head>
<body>
    <h2>Bjarne - C/C++ Assistant</h2>
    <div id="chat"></div>
    <div id="input-area">
        <input type="text" id="prompt" placeholder="What would you have me create?" />
        <button onclick="send()">Send</button>
    </div>
    <script>
        const vscode = acquireVsCodeApi();
        const chat = document.getElementById('chat');
        const promptInput = document.getElementById('prompt');

        function send() {
            const text = promptInput.value.trim();
            if (!text) return;

            addMessage('user', text);
            vscode.postMessage({ command: 'send', text: text });
            promptInput.value = '';
        }

        function addMessage(role, text) {
            const div = document.createElement('div');
            div.className = 'message ' + role;
            div.textContent = text;
            chat.appendChild(div);
            chat.scrollTop = chat.scrollHeight;
        }

        promptInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') send();
        });

        window.addEventListener('message', (e) => {
            const message = e.data;
            if (message.command === 'response') {
                addMessage('assistant', message.text);
            }
        });
    </script>
</body>
</html>`;
}
