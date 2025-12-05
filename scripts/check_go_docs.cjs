#!/usr/bin/env node
// Simple Go doc coverage checker (CommonJS)

const fs = require('fs');
const path = require('path');

const root = path.join(__dirname, '..');

function walk(dir) {
    const files = fs.readdirSync(dir);
    let results = [];
    for (const file of files) {
        const full = path.join(dir, file);
        const stat = fs.statSync(full);
        if (stat.isDirectory()) {
            if (file === 'vendor' || file === '.git') continue;
            results = results.concat(walk(full));
        } else if (file.endsWith('.go')) {
            results.push(full);
        }
    }
    return results;
}

function report() {
    const files = walk(path.join(root, 'backend'));
    const missing = {};
    const regexType = /^\s*type\s+([A-Z][A-Za-z0-9_]*)\b/;
    const regexFunc = /^\s*func\s+(?:\([^)]+\)\s*)?([A-Z][A-Za-z0-9_]*)\s*\(/;
    const regexVar = /^\s*(?:var|const)\s+([A-Z][A-Za-z0-9_]*)\b/;

    files.forEach(file => {
        const content = fs.readFileSync(file, 'utf8');
        const lines = content.split(/\r?\n/);
        let inBacktick = false;
        for (let i = 0; i < lines.length; i++) {
            const line = lines[i];
            // toggle backtick string state when encountering backtick character
            if (line.includes("`") && !line.trim().startsWith("//")) {
                inBacktick = !inBacktick;
            }
            if (inBacktick) continue; // skip matches inside raw backtick strings
            let m;
            if ((m = line.match(regexType))) {
                const name = m[1];
                // Check previous non-empty line for comment containing name at start
                let k = i - 1;
                // Walk back through contiguous comment block, if any
                let firstCommentIdx = -1;
                while(k >= 0 && lines[k].trim().startsWith('//')) { firstCommentIdx = k; k--; }
                // Skip any whitespace immediately above comment block
                while(k >= 0 && lines[k].trim() === '') k--;
                const prev = firstCommentIdx >= 0 ? lines[firstCommentIdx].trim() : (k >= 0 ? lines[k].trim() : '');
                const hasComment = prev.startsWith('// ' + name) || prev.startsWith('// ' + name + ' ');
                if (!hasComment) {
                    if (!missing[file]) missing[file] = [];
                    missing[file].push({line: i+1, type: 'type', name});
                }
            }
            if ((m = line.match(regexFunc))) {
                const name = m[1];
                let k = i - 1;
                let firstCommentIdx = -1;
                while(k >= 0 && lines[k].trim().startsWith('//')) { firstCommentIdx = k; k--; }
                while(k >= 0 && lines[k].trim() === '') k--;
                const prev = firstCommentIdx >= 0 ? lines[firstCommentIdx].trim() : (k >= 0 ? lines[k].trim() : '');
                const hasComment = prev.startsWith('// ' + name) || prev.startsWith('// ' + name + ' ');
                if (!hasComment) {
                    if (!missing[file]) missing[file] = [];
                    missing[file].push({line: i+1, type: 'func', name});
                }
            }
            if ((m = line.match(regexVar))) {
                const name = m[1];
                let k = i - 1;
                let firstCommentIdx = -1;
                while(k >= 0 && lines[k].trim().startsWith('//')) { firstCommentIdx = k; k--; }
                while(k >= 0 && lines[k].trim() === '') k--;
                const prev = firstCommentIdx >= 0 ? lines[firstCommentIdx].trim() : (k >= 0 ? lines[k].trim() : '');
                const hasComment = prev.startsWith('// ' + name) || prev.startsWith('// ' + name + ' ');
                if (!hasComment) {
                    if (!missing[file]) missing[file] = [];
                    missing[file].push({line: i+1, type: 'var/const', name});
                }
            }
        }
    });

    if (Object.keys(missing).length === 0) {
        console.log('No missing doc comments found (per simple heuristic).');
        process.exit(0);
    }

    console.log('Potential missing doc comments:');
    for (const file of Object.keys(missing)) {
        console.log('\n' + file + ':');
        missing[file].forEach(item => {
            console.log(`  - ${item.type} ${item.name} at line ${item.line}`);
        });
    }
}

report();
