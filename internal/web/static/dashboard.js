(function() {
    'use strict';

    // ============================================
    // EXPAND BUTTON HANDLER
    // ============================================
    document.addEventListener('click', function(e) {
        var btn = e.target.closest('.expand-btn');
        if (!btn) return;

        e.preventDefault();
        var panel = btn.closest('.panel');
        if (!panel) return;

        if (panel.classList.contains('expanded')) {
            panel.classList.remove('expanded');
            btn.textContent = 'Expand';
        } else {
            document.querySelectorAll('.panel.expanded').forEach(function(p) {
                p.classList.remove('expanded');
                var b = p.querySelector('.expand-btn');
                if (b) b.textContent = 'Expand';
            });
            panel.classList.add('expanded');
            btn.textContent = '✕ Close';
        }
    });

    // After HTMX swap, close any expanded panels
    document.body.addEventListener('htmx:afterSwap', function() {
        document.querySelectorAll('.panel.expanded').forEach(function(p) {
            p.classList.remove('expanded');
        });
    });

    // ============================================
    // COMMAND PALETTE
    // ============================================
    var allCommands = [];
    var visibleCommands = [];
    var selectedIdx = 0;
    var isPaletteOpen = false;
    var executionLock = false;
    var pendingCommand = null; // Command waiting for args
    var cachedOptions = null;  // Cached options from /api/options

    var overlay = document.getElementById('command-palette-overlay');
    var searchInput = document.getElementById('command-palette-input');
    var resultsDiv = document.getElementById('command-palette-results');
    var toastContainer = document.getElementById('toast-container');
    var outputPanel = document.getElementById('output-panel');
    var outputContent = document.getElementById('output-panel-content');
    var outputCmd = document.getElementById('output-panel-cmd');

    // Output panel
    function showOutput(cmd, output) {
        outputCmd.textContent = 'gt ' + cmd;
        outputContent.textContent = output;
        outputPanel.classList.add('open');
    }

    document.getElementById('output-close-btn').onclick = function() {
        outputPanel.classList.remove('open');
    };

    document.getElementById('output-copy-btn').onclick = function() {
        navigator.clipboard.writeText(outputContent.textContent).then(function() {
            showToast('success', 'Copied', 'Output copied to clipboard');
        });
    };

    // Load commands once
    fetch('/api/commands')
        .then(function(r) { return r.json(); })
        .then(function(data) {
            allCommands = data.commands || [];
        })
        .catch(function() {
            console.error('Failed to load commands');
        });

    // Fetch dynamic options (rigs, polecats, convoys, agents, hooks)
    function fetchOptions() {
        return fetch('/api/options')
            .then(function(r) { return r.json(); })
            .then(function(data) {
                cachedOptions = data;
                return data;
            })
            .catch(function() {
                console.error('Failed to load options');
                return null;
            });
    }

    // Get options for a specific argType
    // Returns array of {value, label, disabled} objects
    function getOptionsForType(argType) {
        if (!cachedOptions) return [];

        var rawOptions;
        switch (argType) {
            case 'rigs': rawOptions = cachedOptions.rigs || []; break;
            case 'polecats': rawOptions = cachedOptions.polecats || []; break;
            case 'convoys': rawOptions = cachedOptions.convoys || []; break;
            case 'agents': rawOptions = cachedOptions.agents || []; break;
            case 'hooks': rawOptions = cachedOptions.hooks || []; break;
            case 'messages': rawOptions = cachedOptions.messages || []; break;
            case 'crew': rawOptions = cachedOptions.crew || []; break;
            case 'escalations': rawOptions = cachedOptions.escalations || []; break;
            default: return [];
        }

        // Normalize to {value, label, disabled} format
        return rawOptions.map(function(opt) {
            if (typeof opt === 'string') {
                return { value: opt, label: opt, disabled: false };
            }
            // Agent format: {name, status, running}
            var statusText = opt.running ? '● running' : '○ stopped';
            return {
                value: opt.name,
                label: opt.name + ' (' + statusText + ')',
                disabled: !opt.running,
                running: opt.running
            };
        });
    }

    function escapeHtml(str) {
        if (!str) return '';
        var div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    // Parse args template like "<address> -s <subject> -m <message>" into field definitions
    // Returns [{name: "address", flag: null}, {name: "subject", flag: "-s"}, {name: "message", flag: "-m"}]
    function parseArgsTemplate(argsStr) {
        if (!argsStr) return [];
        var args = [];
        // Match patterns like "<name>" or "-f <name>"
        var regex = /(?:(-\w+)\s+)?<([^>]+)>/g;
        var match;
        while ((match = regex.exec(argsStr)) !== null) {
            args.push({ name: match[2], flag: match[1] || null });
        }
        return args;
    }

    function renderResults() {
        // If waiting for args, show the args input with options
        if (pendingCommand) {
            var options = pendingCommand.argType ? getOptionsForType(pendingCommand.argType) : [];
            var argFields = parseArgsTemplate(pendingCommand.args);

            var formHtml = '<div class="command-args-prompt">' +
                '<div class="command-args-header">gt ' + escapeHtml(pendingCommand.name) + '</div>';

            // Build form fields for each argument
            for (var i = 0; i < argFields.length; i++) {
                var field = argFields[i];
                var fieldId = 'arg-field-' + i;
                var isFirstField = (i === 0) && !field.flag; // First positional arg
                var hasOptions = isFirstField && pendingCommand.argType && options.length > 0;
                var noOptions = isFirstField && pendingCommand.argType && options.length === 0;
                var isMessageField = field.name === 'message' || field.name === 'body';

                formHtml += '<div class="command-field">';
                formHtml += '<label class="command-field-label" for="' + fieldId + '">' + escapeHtml(field.name) + '</label>';

                if (hasOptions) {
                    // Dropdown for first arg when options exist
                    formHtml += '<select id="' + fieldId + '" class="command-field-select" data-flag="' + (field.flag || '') + '">';
                    formHtml += '<option value="">Select ' + escapeHtml(field.name) + '...</option>';
                    for (var j = 0; j < options.length; j++) {
                        var opt = options[j];
                        var disabledAttr = opt.disabled ? ' disabled' : '';
                        var optClass = opt.disabled ? ' class="option-disabled"' : (opt.running ? ' class="option-running"' : '');
                        formHtml += '<option value="' + escapeHtml(opt.value) + '"' + disabledAttr + optClass + '>' + escapeHtml(opt.label) + '</option>';
                    }
                    formHtml += '</select>';
                } else if (noOptions) {
                    formHtml += '<input type="text" id="' + fieldId + '" class="command-field-input" data-flag="' + (field.flag || '') + '" placeholder="No ' + escapeHtml(pendingCommand.argType) + ' available">';
                } else if (isMessageField) {
                    formHtml += '<textarea id="' + fieldId + '" class="command-field-textarea" data-flag="' + (field.flag || '') + '" placeholder="Enter ' + escapeHtml(field.name) + '..." rows="3"></textarea>';
                } else {
                    formHtml += '<input type="text" id="' + fieldId + '" class="command-field-input" data-flag="' + (field.flag || '') + '" placeholder="Enter ' + escapeHtml(field.name) + '...">';
                }
                formHtml += '</div>';
            }

            // If no arg fields parsed, show generic input
            if (argFields.length === 0 && pendingCommand.args) {
                formHtml += '<div class="command-field">';
                formHtml += '<input type="text" id="arg-field-0" class="command-field-input" placeholder="' + escapeHtml(pendingCommand.args) + '">';
                formHtml += '</div>';
            }

            formHtml += '<div class="command-args-actions">' +
                '<button id="command-args-run" class="command-args-btn run">Run</button>' +
                '<button id="command-args-cancel" class="command-args-btn cancel">Cancel</button>' +
                '</div></div>';

            resultsDiv.innerHTML = formHtml;

            // Focus first field
            var firstField = resultsDiv.querySelector('#arg-field-0');
            if (firstField) firstField.focus();

            // Wire up run/cancel buttons
            var runBtn = document.getElementById('command-args-run');
            var cancelBtn = document.getElementById('command-args-cancel');

            if (runBtn) {
                runBtn.onclick = function() {
                    runBtn.classList.add('loading');
                    runBtn.textContent = 'Running';
                    runWithArgsFromForm(argFields.length || 1);
                };
            }
            if (cancelBtn) {
                cancelBtn.onclick = cancelArgs;
            }

            // Enter key submits
            resultsDiv.querySelectorAll('input, select').forEach(function(el) {
                el.onkeydown = function(e) {
                    if (e.key === 'Enter') {
                        e.preventDefault();
                        runWithArgsFromForm(argFields.length || 1);
                    } else if (e.key === 'Escape') {
                        e.preventDefault();
                        cancelArgs();
                    }
                };
            });
            return;
        }

        if (visibleCommands.length === 0) {
            resultsDiv.innerHTML = '<div class="command-palette-empty">No matching commands</div>';
            return;
        }
        var html = '';
        for (var i = 0; i < visibleCommands.length; i++) {
            var cmd = visibleCommands[i];
            var cls = 'command-item' + (i === selectedIdx ? ' selected' : '');
            var argsHint = cmd.args ? ' <span class="command-args">' + escapeHtml(cmd.args) + '</span>' : '';
            html += '<div class="' + cls + '" data-cmd-name="' + escapeHtml(cmd.name) + '" data-cmd-args="' + escapeHtml(cmd.args || '') + '">' +
                '<span class="command-name">gt ' + escapeHtml(cmd.name) + argsHint + '</span>' +
                '<span class="command-desc">' + escapeHtml(cmd.desc) + '</span>' +
                '<span class="command-category">' + escapeHtml(cmd.category) + '</span>' +
                '</div>';
        }
        resultsDiv.innerHTML = html;
    }

    function runWithArgsFromForm(fieldCount) {
        var args = [];
        for (var i = 0; i < fieldCount; i++) {
            var field = document.getElementById('arg-field-' + i);
            if (field) {
                var val = field.value.trim();
                var flag = field.getAttribute('data-flag');
                if (val) {
                    if (flag) {
                        // Flag-based arg: -s "value"
                        args.push(flag);
                        args.push('"' + val.replace(/"/g, '\\"') + '"');
                    } else {
                        // Positional arg
                        args.push(val);
                    }
                }
            }
        }
        if (pendingCommand) {
            var fullCmd = pendingCommand.name + (args.length ? ' ' + args.join(' ') : '');
            pendingCommand = null;
            runCommand(fullCmd);
        }
    }

    function runWithArgs() {
        runWithArgsFromForm(10); // fallback
    }

    function cancelArgs() {
        pendingCommand = null;
        filterCommands(searchInput ? searchInput.value : '');
    }

    function filterCommands(query) {
        query = (query || '').toLowerCase();
        if (!query) {
            visibleCommands = allCommands.slice();
        } else {
            visibleCommands = allCommands.filter(function(cmd) {
                return cmd.name.toLowerCase().indexOf(query) !== -1 ||
                       cmd.desc.toLowerCase().indexOf(query) !== -1 ||
                       cmd.category.toLowerCase().indexOf(query) !== -1;
            });
        }
        selectedIdx = 0;
        renderResults();
    }

    function openPalette() {
        isPaletteOpen = true;
        pendingCommand = null;
        if (overlay) {
            overlay.style.display = 'flex';
            overlay.classList.add('open');
        }
        if (searchInput) {
            searchInput.value = '';
            searchInput.focus();
        }
        filterCommands('');
        // Fetch fresh options in background
        fetchOptions();
    }

    function closePalette() {
        isPaletteOpen = false;
        pendingCommand = null;
        if (overlay) {
            overlay.classList.remove('open');
            overlay.style.display = 'none';
        }
        if (searchInput) {
            searchInput.value = '';
        }
        visibleCommands = [];
        if (resultsDiv) {
            resultsDiv.innerHTML = '';
        }
    }

    function selectCommand(cmdName, cmdArgs) {
        // If command needs args, show args input
        if (cmdArgs) {
            var cmd = allCommands.find(function(c) { return c.name === cmdName; });
            if (cmd) {
                pendingCommand = cmd;
                // Make sure options are loaded before rendering
                if (cmd.argType && !cachedOptions) {
                    fetchOptions().then(function() {
                        renderResults();
                    });
                } else {
                    renderResults();
                }
                return;
            }
        }
        // No args needed, run directly
        runCommand(cmdName);
    }

    function runCommand(cmdName) {
        if (executionLock) {
            console.log('Execution locked, ignoring');
            return;
        }
        if (!cmdName) {
            console.log('No command name');
            return;
        }

        // Close palette FIRST before anything else
        closePalette();

        executionLock = true;
        console.log('Running command:', cmdName);

        showToast('info', 'Running...', 'gt ' + cmdName);

        fetch('/api/run', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ command: cmdName })
        })
        .then(function(r) { return r.json(); })
        .then(function(data) {
            if (data.success) {
                showToast('success', 'Success', 'gt ' + cmdName);
                if (data.output && data.output.trim()) {
                    showOutput(cmdName, data.output);
                }
            } else {
                showToast('error', 'Failed', data.error || 'Unknown error');
                if (data.output) {
                    showOutput(cmdName, data.output);
                }
            }
        })
        .catch(function(err) {
            showToast('error', 'Error', err.message || 'Request failed');
        })
        .finally(function() {
            // Unlock after 1 second to prevent double-clicks
            setTimeout(function() {
                executionLock = false;
            }, 1000);
        });
    }

    function showToast(type, title, message) {
        var toast = document.createElement('div');
        toast.className = 'toast ' + type;
        var icon = type === 'success' ? '✓' : type === 'error' ? '✕' : 'ℹ';
        toast.innerHTML = '<span class="toast-icon">' + icon + '</span>' +
            '<div class="toast-content">' +
            '<div class="toast-title">' + escapeHtml(title) + '</div>' +
            '<div class="toast-message">' + escapeHtml(message) + '</div>' +
            '</div>' +
            '<button class="toast-close">✕</button>';
        toastContainer.appendChild(toast);

        setTimeout(function() {
            if (toast.parentNode) toast.parentNode.removeChild(toast);
        }, 4000);

        toast.querySelector('.toast-close').onclick = function() {
            if (toast.parentNode) toast.parentNode.removeChild(toast);
        };
    }

    // SINGLE click handler for command palette
    resultsDiv.addEventListener('click', function(e) {
        var item = e.target.closest('.command-item');
        if (!item) return;

        e.preventDefault();
        e.stopPropagation();

        var cmdName = item.getAttribute('data-cmd-name');
        var cmdArgs = item.getAttribute('data-cmd-args');
        if (cmdName) {
            selectCommand(cmdName, cmdArgs);
        }
    });

    // Open palette button
    document.addEventListener('click', function(e) {
        if (e.target.closest('#open-palette-btn')) {
            e.preventDefault();
            openPalette();
            return;
        }
        // Click on overlay background closes palette
        if (e.target === overlay) {
            closePalette();
        }
    });

    // Keyboard handling
    document.addEventListener('keydown', function(e) {
        // Cmd+K or Ctrl+K toggles palette
        if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
            e.preventDefault();
            if (isPaletteOpen) {
                closePalette();
            } else {
                openPalette();
            }
            return;
        }

        // Rest only when palette is open
        if (!isPaletteOpen) return;

        // If in args mode, let the args input handle keys
        if (pendingCommand) return;

        if (e.key === 'Escape') {
            e.preventDefault();
            closePalette();
            return;
        }

        if (e.key === 'ArrowDown') {
            e.preventDefault();
            if (visibleCommands.length > 0) {
                selectedIdx = Math.min(selectedIdx + 1, visibleCommands.length - 1);
                renderResults();
            }
            return;
        }

        if (e.key === 'ArrowUp') {
            e.preventDefault();
            selectedIdx = Math.max(selectedIdx - 1, 0);
            renderResults();
            return;
        }

        if (e.key === 'Enter') {
            e.preventDefault();
            if (visibleCommands[selectedIdx]) {
                var cmd = visibleCommands[selectedIdx];
                selectCommand(cmd.name, cmd.args);
            }
            return;
        }
    });

    // Input filtering
    searchInput.addEventListener('input', function() {
        filterCommands(searchInput.value);
    });

    // ============================================
    // MAIL PANEL INTERACTIONS
    // ============================================
    var mailList = document.getElementById('mail-list');
    var mailAll = document.getElementById('mail-all');
    var mailDetail = document.getElementById('mail-detail');
    var mailCompose = document.getElementById('mail-compose');
    var currentMessageId = null;
    var currentMessageFrom = null;
    var currentMailTab = 'inbox';

    // Mail tab switching
    document.querySelectorAll('.mail-tab').forEach(function(tab) {
        tab.addEventListener('click', function() {
            var targetTab = tab.getAttribute('data-tab');
            if (targetTab === currentMailTab) return;

            // Update active tab
            document.querySelectorAll('.mail-tab').forEach(function(t) {
                t.classList.remove('active');
            });
            tab.classList.add('active');
            currentMailTab = targetTab;

            // Show/hide views
            if (targetTab === 'inbox') {
                mailList.style.display = 'block';
                mailAll.style.display = 'none';
            } else {
                mailList.style.display = 'none';
                mailAll.style.display = 'block';
            }

            // Hide detail/compose views
            mailDetail.style.display = 'none';
            mailCompose.style.display = 'none';
        });
    });

    // Load mail inbox on page load
    function loadMailInbox() {
        var loading = document.getElementById('mail-loading');
        var table = document.getElementById('mail-table');
        var tbody = document.getElementById('mail-tbody');
        var empty = document.getElementById('mail-empty');
        var count = document.getElementById('mail-count');

        if (!loading || !table || !tbody) return;

        fetch('/api/mail/inbox')
            .then(function(r) { return r.json(); })
            .then(function(data) {
                loading.style.display = 'none';

                if (data.messages && data.messages.length > 0) {
                    table.style.display = 'table';
                    empty.style.display = 'none';
                    tbody.innerHTML = '';

                    data.messages.forEach(function(msg) {
                        var tr = document.createElement('tr');
                        tr.className = 'mail-row' + (msg.read ? '' : ' mail-unread');
                        tr.setAttribute('data-msg-id', msg.id);
                        tr.setAttribute('data-from', msg.from);

                        var priorityIcon = '';
                        if (msg.priority === 'urgent') priorityIcon = '<span class="priority-urgent">⚡</span> ';
                        else if (msg.priority === 'high') priorityIcon = '<span class="priority-high">!</span> ';

                        tr.innerHTML =
                            '<td class="mail-from">' + escapeHtml(msg.from) + '</td>' +
                            '<td>' + priorityIcon + '<span class="mail-subject">' + escapeHtml(msg.subject) + '</span></td>' +
                            '<td class="mail-time">' + formatMailTime(msg.timestamp) + '</td>';
                        tbody.appendChild(tr);
                    });

                    // Update count
                    if (count) {
                        var unread = data.unread_count || 0;
                        count.textContent = unread > 0 ? unread + ' unread' : data.total;
                        if (unread > 0) count.classList.add('has-unread');
                    }
                } else {
                    table.style.display = 'none';
                    empty.style.display = 'block';
                    if (count) count.textContent = '0';
                }
            })
            .catch(function(err) {
                loading.textContent = 'Failed to load mail';
                console.error('Mail load error:', err);
            });
    }

    function formatMailTime(timestamp) {
        if (!timestamp) return '';
        var d = new Date(timestamp);
        var now = new Date();
        var diff = now - d;

        if (diff < 60000) return 'just now';
        if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
        if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
        if (diff < 604800000) return Math.floor(diff / 86400000) + 'd ago';
        return d.toLocaleDateString();
    }

    // Load mail on page load
    loadMailInbox();

    // Click on mail row to read message
    document.addEventListener('click', function(e) {
        var mailRow = e.target.closest('.mail-row');
        if (mailRow) {
            e.preventDefault();
            var msgId = mailRow.getAttribute('data-msg-id');
            var from = mailRow.getAttribute('data-from');
            if (msgId) {
                openMailDetail(msgId, from);
            }
        }
    });

    function openMailDetail(msgId, from) {
        currentMessageId = msgId;
        currentMessageFrom = from;

        // Pause HTMX refresh while viewing/composing mail
        window.pauseRefresh = true;

        // Show loading state
        document.getElementById('mail-detail-subject').textContent = 'Loading...';
        document.getElementById('mail-detail-from').textContent = from || '';
        document.getElementById('mail-detail-body').textContent = '';
        document.getElementById('mail-detail-time').textContent = '';

        // Hide both list views and compose, show detail
        mailList.style.display = 'none';
        if (mailAll) mailAll.style.display = 'none';
        mailCompose.style.display = 'none';
        mailDetail.style.display = 'block';

        // Fetch message content
        fetch('/api/mail/read?id=' + encodeURIComponent(msgId))
            .then(function(r) { return r.json(); })
            .then(function(msg) {
                document.getElementById('mail-detail-subject').textContent = msg.subject || '(no subject)';
                document.getElementById('mail-detail-from').textContent = msg.from || from;
                document.getElementById('mail-detail-body').textContent = msg.body || '(no content)';
                document.getElementById('mail-detail-time').textContent = msg.timestamp || '';
            })
            .catch(function(err) {
                document.getElementById('mail-detail-body').textContent = 'Error loading message: ' + err.message;
            });
    }

    // Back button from detail view - return to correct tab
    document.getElementById('mail-back-btn').addEventListener('click', function() {
        mailDetail.style.display = 'none';
        mailCompose.style.display = 'none';

        // Return to the correct view based on current tab
        if (currentMailTab === 'all' && mailAll) {
            mailAll.style.display = 'block';
            mailList.style.display = 'none';
        } else {
            mailList.style.display = 'block';
            if (mailAll) mailAll.style.display = 'none';
        }

        currentMessageId = null;
        currentMessageFrom = null;
        // Resume HTMX refresh
        window.pauseRefresh = false;
    });

    // Reply button
    document.getElementById('mail-reply-btn').addEventListener('click', function() {
        var subject = document.getElementById('mail-detail-subject').textContent;
        var replySubject = subject.startsWith('Re: ') ? subject : 'Re: ' + subject;

        document.getElementById('mail-compose-title').textContent = 'Reply';
        document.getElementById('compose-subject').value = replySubject;
        document.getElementById('compose-reply-to').value = currentMessageId || '';
        document.getElementById('compose-body').value = '';

        // Populate To dropdown and select the sender
        populateToDropdown(currentMessageFrom);

        mailDetail.style.display = 'none';
        mailCompose.style.display = 'block';
        document.getElementById('compose-body').focus();
    });

    // Compose new message button
    document.getElementById('compose-mail-btn').addEventListener('click', function() {
        // Pause HTMX refresh while composing
        window.pauseRefresh = true;

        document.getElementById('mail-compose-title').textContent = 'New Message';
        document.getElementById('compose-subject').value = '';
        document.getElementById('compose-body').value = '';
        document.getElementById('compose-reply-to').value = '';

        // Populate To dropdown
        populateToDropdown(null);

        mailList.style.display = 'none';
        mailDetail.style.display = 'none';
        mailCompose.style.display = 'block';
        document.getElementById('compose-to').focus();
    });

    // Back button from compose view
    document.getElementById('compose-back-btn').addEventListener('click', function() {
        mailCompose.style.display = 'none';
        if (currentMessageId) {
            mailDetail.style.display = 'block';
        } else {
            mailList.style.display = 'block';
        }
    });

    // Cancel compose
    document.getElementById('compose-cancel-btn').addEventListener('click', function() {
        mailCompose.style.display = 'none';
        mailList.style.display = 'block';
        currentMessageId = null;
        currentMessageFrom = null;
        // Resume HTMX refresh
        window.pauseRefresh = false;
    });

    // Send message
    document.getElementById('mail-send-btn').addEventListener('click', function() {
        var to = document.getElementById('compose-to').value;
        var subject = document.getElementById('compose-subject').value;
        var body = document.getElementById('compose-body').value;
        var replyTo = document.getElementById('compose-reply-to').value;

        if (!to || !subject) {
            showToast('error', 'Missing fields', 'Please fill in To and Subject');
            return;
        }

        var btn = document.getElementById('mail-send-btn');
        btn.textContent = 'Sending...';
        btn.disabled = true;

        fetch('/api/mail/send', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                to: to,
                subject: subject,
                body: body,
                reply_to: replyTo || undefined
            })
        })
        .then(function(r) { return r.json(); })
        .then(function(data) {
            if (data.success) {
                showToast('success', 'Sent', 'Message sent to ' + to);
                mailCompose.style.display = 'none';
                mailList.style.display = 'block';
                currentMessageId = null;
                currentMessageFrom = null;
                // Resume HTMX refresh and reload inbox
                window.pauseRefresh = false;
                loadMailInbox();
            } else {
                showToast('error', 'Failed', data.error || 'Failed to send message');
            }
        })
        .catch(function(err) {
            showToast('error', 'Error', err.message);
        })
        .finally(function() {
            btn.textContent = 'Send';
            btn.disabled = false;
        });
    });

    // Populate To dropdown with agents
    // Returns a Promise so callers can wait for it
    function populateToDropdown(selectedValue) {
        var select = document.getElementById('compose-to');
        select.innerHTML = '<option value="">Select recipient...</option>';

        // If we have a selected value for reply, add it immediately so it's available
        if (selectedValue) {
            var cleanValue = selectedValue.replace(/\/$/, '').trim();
            var opt = document.createElement('option');
            opt.value = cleanValue;
            opt.textContent = cleanValue + ' (replying to)';
            opt.selected = true;
            select.appendChild(opt);
        }

        // Fetch agents from options API
        return fetch('/api/options')
            .then(function(r) { return r.json(); })
            .then(function(data) {
                var agents = data.agents || [];
                var addedValues = selectedValue ? [selectedValue.replace(/\/$/, '').toLowerCase()] : [];

                agents.forEach(function(agent) {
                    var name = typeof agent === 'string' ? agent : agent.name;
                    var running = typeof agent === 'object' ? agent.running : true;

                    // Skip if already added as reply-to
                    if (addedValues.indexOf(name.toLowerCase()) !== -1) {
                        return;
                    }

                    var opt = document.createElement('option');
                    opt.value = name;
                    opt.textContent = name + (running ? ' (● running)' : ' (○ stopped)');
                    if (!running) opt.disabled = true;
                    select.appendChild(opt);
                });
            })
            .catch(function(err) {
                console.error('Failed to load agents for To dropdown:', err);
            });
    }

    // ============================================
    // ISSUE PANEL INTERACTIONS
    // ============================================
    var issuesList = document.getElementById('issues-list');
    var issueDetail = document.getElementById('issue-detail');
    var currentIssueId = null;

    // Click on issue row to view details
    document.addEventListener('click', function(e) {
        var issueRow = e.target.closest('.issue-row');
        if (issueRow && issueRow.hasAttribute('data-issue-id')) {
            e.preventDefault();
            var issueId = issueRow.getAttribute('data-issue-id');
            if (issueId) {
                openIssueDetail(issueId);
            }
        }

        // Click on dependency links
        var depItem = e.target.closest('.issue-dep-item');
        if (depItem) {
            e.preventDefault();
            var depId = depItem.getAttribute('data-issue-id');
            if (depId) {
                openIssueDetail(depId);
            }
        }
    });

    function openIssueDetail(issueId) {
        currentIssueId = issueId;

        // Pause HTMX refresh while viewing issue
        window.pauseRefresh = true;

        // Show loading state
        document.getElementById('issue-detail-id').textContent = issueId;
        document.getElementById('issue-detail-title-text').textContent = 'Loading...';
        document.getElementById('issue-detail-description').textContent = '';
        document.getElementById('issue-detail-priority').textContent = '';
        document.getElementById('issue-detail-status').textContent = '';
        document.getElementById('issue-detail-type').textContent = '';
        document.getElementById('issue-detail-created').textContent = '';
        document.getElementById('issue-detail-depends-on').innerHTML = '';
        document.getElementById('issue-detail-blocks').innerHTML = '';
        document.getElementById('issue-detail-deps').style.display = 'none';
        document.getElementById('issue-detail-blocks-section').style.display = 'none';

        // Show detail view
        issuesList.style.display = 'none';
        issueDetail.style.display = 'block';

        // Fetch issue details
        fetch('/api/issues/show?id=' + encodeURIComponent(issueId))
            .then(function(r) { return r.json(); })
            .then(function(data) {
                if (data.error) {
                    document.getElementById('issue-detail-title-text').textContent = 'Error loading issue';
                    document.getElementById('issue-detail-description').textContent = data.error;
                    return;
                }

                document.getElementById('issue-detail-id').textContent = data.id || issueId;
                document.getElementById('issue-detail-title-text').textContent = data.title || '(no title)';
                document.getElementById('issue-detail-description').textContent = data.description || data.raw_output || '(no description)';

                // Priority badge
                var priorityEl = document.getElementById('issue-detail-priority');
                if (data.priority) {
                    priorityEl.textContent = data.priority;
                    priorityEl.className = 'badge';
                    if (data.priority === 'P1') priorityEl.classList.add('badge-red');
                    else if (data.priority === 'P2') priorityEl.classList.add('badge-orange');
                    else if (data.priority === 'P3') priorityEl.classList.add('badge-yellow');
                    else priorityEl.classList.add('badge-muted');
                }

                // Status
                var statusEl = document.getElementById('issue-detail-status');
                if (data.status) {
                    statusEl.textContent = data.status;
                    statusEl.className = 'issue-status ' + data.status.toLowerCase().replace(' ', '_');
                }

                // Meta info
                if (data.type) {
                    document.getElementById('issue-detail-type').textContent = 'Type: ' + data.type;
                }
                if (data.created) {
                    document.getElementById('issue-detail-created').textContent = 'Created: ' + data.created;
                }

                // Dependencies
                if (data.depends_on && data.depends_on.length > 0) {
                    document.getElementById('issue-detail-deps').style.display = 'block';
                    var depsHtml = data.depends_on.map(function(dep) {
                        return '<span class="issue-dep-item" data-issue-id="' + escapeHtml(dep) + '">→ ' + escapeHtml(dep) + '</span>';
                    }).join(' ');
                    document.getElementById('issue-detail-depends-on').innerHTML = depsHtml;
                }

                // Blocks
                if (data.blocks && data.blocks.length > 0) {
                    document.getElementById('issue-detail-blocks-section').style.display = 'block';
                    var blocksHtml = data.blocks.map(function(dep) {
                        return '<span class="issue-dep-item" data-issue-id="' + escapeHtml(dep) + '">← ' + escapeHtml(dep) + '</span>';
                    }).join(' ');
                    document.getElementById('issue-detail-blocks').innerHTML = blocksHtml;
                }
            })
            .catch(function(err) {
                document.getElementById('issue-detail-title-text').textContent = 'Error';
                document.getElementById('issue-detail-description').textContent = 'Failed to load issue: ' + err.message;
            });
    }

    // Back button from issue detail
    var issueBackBtn = document.getElementById('issue-back-btn');
    if (issueBackBtn) {
        issueBackBtn.addEventListener('click', function() {
            issueDetail.style.display = 'none';
            issuesList.style.display = 'block';
            currentIssueId = null;
            // Resume HTMX refresh
            window.pauseRefresh = false;
        });
    }

    // ============================================
    // PR/MERGE QUEUE PANEL INTERACTIONS
    // ============================================
    var prList = document.getElementById('pr-list');
    var prDetail = document.getElementById('pr-detail');
    var currentPrUrl = null;

    // Click on PR row to view details
    document.addEventListener('click', function(e) {
        var prRow = e.target.closest('.pr-row');
        if (prRow && prRow.hasAttribute('data-pr-url')) {
            e.preventDefault();
            var prUrl = prRow.getAttribute('data-pr-url');
            if (prUrl) {
                openPrDetail(prUrl);
            }
        }
    });

    function openPrDetail(prUrl) {
        currentPrUrl = prUrl;

        // Pause HTMX refresh while viewing PR
        window.pauseRefresh = true;

        // Show loading state
        document.getElementById('pr-detail-number').textContent = 'Loading...';
        document.getElementById('pr-detail-title-text').textContent = '';
        document.getElementById('pr-detail-body').textContent = '';
        document.getElementById('pr-detail-state').textContent = '';
        document.getElementById('pr-detail-author').textContent = '';
        document.getElementById('pr-detail-branches').textContent = '';
        document.getElementById('pr-detail-created').textContent = '';
        document.getElementById('pr-detail-additions').textContent = '';
        document.getElementById('pr-detail-deletions').textContent = '';
        document.getElementById('pr-detail-files').textContent = '';
        document.getElementById('pr-detail-labels').innerHTML = '';
        document.getElementById('pr-detail-checks').innerHTML = '';
        document.getElementById('pr-detail-labels-section').style.display = 'none';
        document.getElementById('pr-detail-checks-section').style.display = 'none';
        document.getElementById('pr-detail-link').href = prUrl;

        // Show detail view
        prList.style.display = 'none';
        prDetail.style.display = 'block';

        // Fetch PR details
        fetch('/api/pr/show?url=' + encodeURIComponent(prUrl))
            .then(function(r) { return r.json(); })
            .then(function(data) {
                if (data.error) {
                    document.getElementById('pr-detail-title-text').textContent = 'Error loading PR';
                    document.getElementById('pr-detail-body').textContent = data.error;
                    return;
                }

                document.getElementById('pr-detail-number').textContent = '#' + data.number;
                document.getElementById('pr-detail-title-text').textContent = data.title || '(no title)';
                document.getElementById('pr-detail-body').textContent = data.body || '(no description)';

                // State badge
                var stateEl = document.getElementById('pr-detail-state');
                if (data.state) {
                    stateEl.textContent = data.state;
                    stateEl.className = 'pr-state ' + data.state.toLowerCase();
                }

                // Meta info
                if (data.author) {
                    document.getElementById('pr-detail-author').textContent = 'by ' + data.author;
                }
                if (data.base_ref && data.head_ref) {
                    document.getElementById('pr-detail-branches').textContent = data.head_ref + ' → ' + data.base_ref;
                }
                if (data.created_at) {
                    var created = new Date(data.created_at);
                    document.getElementById('pr-detail-created').textContent = 'Created ' + created.toLocaleDateString();
                }

                // Stats
                if (data.additions !== undefined) {
                    document.getElementById('pr-detail-additions').textContent = '+' + data.additions;
                }
                if (data.deletions !== undefined) {
                    document.getElementById('pr-detail-deletions').textContent = '-' + data.deletions;
                }
                if (data.changed_files !== undefined) {
                    document.getElementById('pr-detail-files').textContent = data.changed_files + ' files';
                }

                // Labels
                if (data.labels && data.labels.length > 0) {
                    document.getElementById('pr-detail-labels-section').style.display = 'block';
                    var labelsHtml = data.labels.map(function(label) {
                        return '<span class="pr-label">' + escapeHtml(label) + '</span>';
                    }).join(' ');
                    document.getElementById('pr-detail-labels').innerHTML = labelsHtml;
                }

                // Checks
                if (data.checks && data.checks.length > 0) {
                    document.getElementById('pr-detail-checks-section').style.display = 'block';
                    var checksHtml = data.checks.map(function(check) {
                        var checkClass = 'pr-check';
                        if (check.toLowerCase().includes('success')) checkClass += ' success';
                        else if (check.toLowerCase().includes('failure')) checkClass += ' failure';
                        else if (check.toLowerCase().includes('pending') || check.toLowerCase().includes('in_progress')) checkClass += ' pending';
                        return '<span class="' + checkClass + '">' + escapeHtml(check) + '</span>';
                    }).join('');
                    document.getElementById('pr-detail-checks').innerHTML = checksHtml;
                }
            })
            .catch(function(err) {
                document.getElementById('pr-detail-title-text').textContent = 'Error';
                document.getElementById('pr-detail-body').textContent = 'Failed to load PR: ' + err.message;
            });
    }

    // Back button from PR detail
    var prBackBtn = document.getElementById('pr-back-btn');
    if (prBackBtn) {
        prBackBtn.addEventListener('click', function() {
            prDetail.style.display = 'none';
            prList.style.display = 'block';
            currentPrUrl = null;
            // Resume HTMX refresh
            window.pauseRefresh = false;
        });
    }

})();
