// Extension Chili : souligne les erreurs renvoyées par « chili check ».
//
// Le contenu de l'éditeur (même non enregistré) est écrit dans un fichier
// temporaire, puis « chili check » est lancé dessus. Chaque ligne d'erreur
// au format « fichier:ligne:col: message » devient un diagnostic.

const vscode = require('vscode');
const cp = require('child_process');
const fs = require('fs');
const os = require('os');
const path = require('path');
const crypto = require('crypto');

const DEBOUNCE_MS = 400;
const ERROR_LINE = /^(.*):(\d+):(\d+): (.*)$/;

let diagnostics;
let tmpDir;
const timers = new Map();   // uri -> timer du debounce
const running = new Map();  // uri -> processus en cours
let missingWarned = false;

function activate(context) {
    diagnostics = vscode.languages.createDiagnosticCollection('chili');
    context.subscriptions.push(diagnostics);

    tmpDir = path.join(os.tmpdir(), 'chili-vscode');
    try { fs.mkdirSync(tmpDir, { recursive: true }); } catch (e) { /* vérifié à l'écriture */ }

    context.subscriptions.push(
        vscode.workspace.onDidOpenTextDocument(doc => schedule(doc, 0)),
        vscode.workspace.onDidSaveTextDocument(doc => schedule(doc, 0)),
        vscode.workspace.onDidChangeTextDocument(ev => schedule(ev.document, DEBOUNCE_MS)),
        vscode.workspace.onDidCloseTextDocument(doc => forget(doc)),
        vscode.workspace.onDidChangeConfiguration(ev => {
            if (ev.affectsConfiguration('chili')) {
                missingWarned = false;
                vscode.workspace.textDocuments.forEach(doc => schedule(doc, 0));
            }
        })
    );

    vscode.workspace.textDocuments.forEach(doc => schedule(doc, 0));
}

function deactivate() {
    timers.forEach(t => clearTimeout(t));
    running.forEach(p => p.kill());
}

function schedule(doc, delay) {
    if (doc.languageId !== 'chili') return;
    const key = doc.uri.toString();
    clearTimeout(timers.get(key));
    timers.set(key, setTimeout(() => {
        timers.delete(key);
        check(doc);
    }, delay));
}

function forget(doc) {
    const key = doc.uri.toString();
    clearTimeout(timers.get(key));
    timers.delete(key);
    const proc = running.get(key);
    if (proc) proc.kill();
    running.delete(key);
    diagnostics.delete(doc.uri);
    fs.unlink(tmpFile(doc), () => {});
}

function tmpFile(doc) {
    const hash = crypto.createHash('sha1').update(doc.uri.toString()).digest('hex').slice(0, 12);
    return path.join(tmpDir, hash + '.chl');
}

function check(doc) {
    if (doc.isClosed) return;
    const config = vscode.workspace.getConfiguration('chili', doc.uri);
    if (!config.get('check.enabled', true)) {
        diagnostics.delete(doc.uri);
        return;
    }

    const key = doc.uri.toString();
    const previous = running.get(key);
    if (previous) previous.kill();

    const file = tmpFile(doc);
    const version = doc.version;
    try {
        fs.writeFileSync(file, doc.getText(), 'utf8');
    } catch (e) {
        return;
    }

    const exe = (config.get('path') || '').trim() || 'chili';
    // Lancé depuis le dossier temporaire avec un nom relatif : pas de « C: » dans les chemins renvoyés.
    const proc = cp.execFile(exe, ['check', path.basename(file)], {
        cwd: tmpDir,
        windowsHide: true,
        timeout: 10000,
        encoding: 'utf8'
    }, (err, stdout, stderr) => {
        if (running.get(key) === proc) running.delete(key);
        if (err && err.code === 'ENOENT') {
            warnMissing(exe);
            return;
        }
        if (err && err.killed) return; // remplacé par une vérification plus récente
        if (doc.isClosed || doc.version !== version) return;
        diagnostics.set(doc.uri, parse(doc, stderr + '\n' + stdout));
    });
    running.set(key, proc);
}

function parse(doc, output) {
    const result = [];
    for (const raw of output.split(/\r?\n/)) {
        const m = ERROR_LINE.exec(raw.trim());
        if (!m) continue;
        const line = Math.max(0, Math.min(parseInt(m[2], 10) - 1, doc.lineCount - 1));
        const text = doc.lineAt(line).text;
        const col = Math.max(0, Math.min(parseInt(m[3], 10) - 1, text.length));
        const range = wordRange(doc, line, col, text);
        const d = new vscode.Diagnostic(range, m[4], vscode.DiagnosticSeverity.Error);
        d.source = 'chili';
        result.push(d);
    }
    return result;
}

// Souligne le mot (ou le caractère) à la position de l'erreur ; en fin de ligne, le dernier caractère.
function wordRange(doc, line, col, text) {
    const pos = new vscode.Position(line, col);
    const word = doc.getWordRangeAtPosition(pos);
    if (word) return word;
    if (col < text.length) return new vscode.Range(line, col, line, col + 1);
    if (text.length > 0) return new vscode.Range(line, text.length - 1, line, text.length);
    return new vscode.Range(line, 0, line, 0);
}

function warnMissing(exe) {
    if (missingWarned) return;
    missingWarned = true;
    vscode.window.showWarningMessage(
        `Chili : commande « ${exe} » introuvable, les erreurs ne seront pas soulignées. ` +
        'Installe Chili ou règle « chili.path ».'
    );
}

module.exports = { activate, deactivate };
