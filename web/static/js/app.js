document.addEventListener('DOMContentLoaded', function() {
    var form = document.getElementById('deploy-form');
    var formSection = document.getElementById('deploy-form-section');
    var dashboardSection = document.getElementById('dashboard-section');
    var appContainer = document.getElementById('app-container');
    var logOutput = document.getElementById('log-output');
    var statusBadge = document.getElementById('status-badge');
    var deployBtn = document.getElementById('deploy-btn');
    var newDeployBtn = document.getElementById('new-deploy-btn');
    var stageList = document.getElementById('stage-list');
    var stageCounter = document.getElementById('stage-counter');
    var liveDot = document.getElementById('live-dot');

    // --- Auth elements ---
    var authSection = document.getElementById('auth-section');
    var authTokenInput = document.getElementById('auth_token');
    var authBtn = document.getElementById('auth-btn');
    var authError = document.getElementById('auth-error');
    var authToken = sessionStorage.getItem('sb_auth_token') || '';

    // SSL mode toggle (segmented control)
    var sslRadios = document.querySelectorAll('input[name="ssl_mode"]');
    var sslLetsencryptOptions = document.getElementById('ssl-letsencrypt-options');
    var sslCustomOptions = document.getElementById('ssl-custom-options');

    sslRadios.forEach(function(radio) {
        radio.addEventListener('change', function() {
            if (this.value === 'letsencrypt') {
                sslLetsencryptOptions.classList.remove('hidden');
                sslCustomOptions.classList.add('hidden');
            } else {
                sslLetsencryptOptions.classList.add('hidden');
                sslCustomOptions.classList.remove('hidden');
            }
        });
    });

    // CloudStack mode toggle (segmented control)
    var csRadios = document.querySelectorAll('input[name="cloudstack_mode"]');
    var csSimulatorOptions = document.getElementById('cloudstack-simulator-options');

    csRadios.forEach(function(radio) {
        radio.addEventListener('change', function() {
            if (this.value === 'simulator') {
                csSimulatorOptions.classList.remove('hidden');
            } else {
                csSimulatorOptions.classList.add('hidden');
            }
        });
    });

    // Initialize floating labels for pre-filled inputs
    initFloatingLabels();

    // ==========================================
    // AUTH FLOW
    // ==========================================

    // Enable auth button when token is entered
    authTokenInput.addEventListener('input', function() {
        authBtn.disabled = !authTokenInput.value.trim();
        authError.style.display = 'none';
    });

    // Submit on Enter key in auth field
    authTokenInput.addEventListener('keypress', function(e) {
        if (e.key === 'Enter' && authTokenInput.value.trim()) verifyToken();
    });

    authBtn.addEventListener('click', verifyToken);

    function verifyToken() {
        var token = authTokenInput.value.trim();
        authBtn.disabled = true;
        authBtn.textContent = 'Verifying...';
        authError.style.display = 'none';

        fetch('/api/deployments', {
            headers: { 'Authorization': 'Bearer ' + token }
        }).then(function(r) {
            if (r.ok) {
                authToken = token;
                sessionStorage.setItem('sb_auth_token', token);
                authSection.classList.add('hidden');
                formSection.classList.remove('hidden');
            } else {
                authError.textContent = 'Invalid access token.';
                authError.style.display = '';
            }
            authBtn.disabled = false;
            authBtn.textContent = 'Continue';
        }).catch(function() {
            authError.textContent = 'Connection failed. Please try again.';
            authError.style.display = '';
            authBtn.disabled = false;
            authBtn.textContent = 'Continue';
        });
    }

    // Auto-verify saved token on page load
    if (authToken) {
        authSection.style.opacity = '0.5';
        fetch('/api/deployments', {
            headers: { 'Authorization': 'Bearer ' + authToken }
        }).then(function(r) {
            authSection.style.opacity = '';
            if (r.ok) {
                authSection.classList.add('hidden');
                formSection.classList.remove('hidden');
            } else {
                sessionStorage.removeItem('sb_auth_token');
                authToken = '';
            }
        }).catch(function() {
            authSection.style.opacity = '';
        });
    }

    function handleAuthFailure() {
        sessionStorage.removeItem('sb_auth_token');
        authToken = '';
        dashboardSection.classList.add('hidden');
        formSection.classList.add('hidden');
        authSection.classList.remove('hidden');
        appContainer.classList.remove('container-wide');
        authError.textContent = 'Session expired. Please re-authenticate.';
        authError.style.display = '';
        authTokenInput.value = '';
        authBtn.disabled = true;
    }

    // ==========================================
    // DEPLOY BUTTON VALIDATION
    // ==========================================

    var requiredFields = form.querySelectorAll('input[required]');

    function validateForm() {
        var allFilled = true;
        requiredFields.forEach(function(input) {
            if (input.closest('.hidden')) return;
            if (!input.value.trim()) allFilled = false;
        });
        deployBtn.disabled = !allFilled;
    }

    requiredFields.forEach(function(input) {
        input.addEventListener('input', validateForm);
        input.addEventListener('change', validateForm);
    });

    sslRadios.forEach(function(radio) {
        radio.addEventListener('change', validateForm);
    });
    csRadios.forEach(function(radio) {
        radio.addEventListener('change', validateForm);
    });

    // ==========================================
    // FORM SUBMISSION
    // ==========================================

    form.addEventListener('submit', async function(e) {
        e.preventDefault();

        var sslMode = document.querySelector('input[name="ssl_mode"]:checked').value;
        var cloudstackMode = document.querySelector('input[name="cloudstack_mode"]:checked').value;

        var payload = {
            server_ip: document.getElementById('server_ip').value,
            ssh_user: document.getElementById('ssh_user').value,
            ssh_pass: document.getElementById('ssh_pass').value,
            ssh_port: parseInt(document.getElementById('ssh_port').value) || 22,
            domain: document.getElementById('domain').value,
            ssl_mode: sslMode,
            letsencrypt_email: document.getElementById('letsencrypt_email').value,
            ssl_cert: document.getElementById('ssl_cert').value,
            ssl_key: document.getElementById('ssl_key').value,
            cloudstack_mode: cloudstackMode,
            cloudstack_version: document.getElementById('cloudstack_version').value,
            ecr_token: document.getElementById('ecr_token').value
        };

        lastPayload = payload;
        deployBtn.disabled = true;
        deployBtn.innerHTML = '<span class="btn-deploying"><span class="spinner"></span> Deploying...</span>';

        try {
            var response = await fetch('/api/deploy', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': 'Bearer ' + authToken
                },
                body: JSON.stringify(payload)
            });

            if (!response.ok) {
                if (response.status === 401) {
                    handleAuthFailure();
                    return;
                }
                var err = await response.json();
                throw new Error(err.error || 'Deployment failed to start');
            }

            var data = await response.json();
            showDashboard(data.id, data.stages);
        } catch (err) {
            alert('Error: ' + err.message);
            deployBtn.disabled = false;
            deployBtn.textContent = 'Deploy StackBill';
        }
    });

    var currentDomain = '';
    var currentServerIP = '';
    var currentDeploymentId = '';
    var lastPayload = null;
    var rawLogLines = [];
    var retryBtn = document.getElementById('retry-btn');

    function showDashboard(deploymentId, stages) {
        currentDomain = document.getElementById('domain').value;
        currentServerIP = document.getElementById('server_ip').value;
        currentDeploymentId = deploymentId;
        formSection.classList.add('hidden');
        dashboardSection.classList.remove('hidden');
        appContainer.classList.add('container-wide');
        logOutput.innerHTML = '';
        rawLogLines = [];
        statusBadge.className = 'badge badge-running';
        statusBadge.textContent = 'Running';
        newDeployBtn.classList.add('hidden');
        retryBtn.classList.add('hidden');
        if (liveDot) liveDot.style.display = '';
        // Clear any previous result
        var oldResult = document.getElementById('result-panel');
        if (oldResult) oldResult.remove();

        renderStages(stages);
        connectSSE(deploymentId);
    }

    // --- SVG Icons ---
    var checkSVG = '<svg width="12" height="12" viewBox="0 0 12 12" fill="none"><path d="M2.5 6L5 8.5L9.5 3.5" stroke="white" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>';
    var checkSVGDark = '<svg width="12" height="12" viewBox="0 0 12 12" fill="none"><path d="M2.5 6L5 8.5L9.5 3.5" stroke="#16A34A" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>';
    var crossSVG = '<svg width="12" height="12" viewBox="0 0 12 12" fill="none"><path d="M3 3L9 9M9 3L3 9" stroke="#DC2626" stroke-width="1.5" stroke-linecap="round"/></svg>';
    var dotSVG = '<svg width="8" height="8" viewBox="0 0 8 8" fill="none"><circle cx="4" cy="4" r="3" fill="currentColor"/></svg>';

    // --- Stage rendering (vertical stepper) ---

    function renderStages(stages) {
        stageList.innerHTML = '';
        for (var i = 0; i < stages.length; i++) {
            var stage = stages[i];
            var div = document.createElement('div');
            div.className = 'stage-item stage-' + stage.status;
            div.id = 'stage-' + i;

            var indicator = document.createElement('div');
            indicator.className = 'stage-indicator';
            indicator.innerHTML = getIndicatorContent(stage.status);

            var name = document.createElement('span');
            name.className = 'stage-name';
            name.textContent = stage.name;

            div.appendChild(indicator);
            div.appendChild(name);
            stageList.appendChild(div);
        }
        stageCounter.textContent = '0 / ' + stages.length;
    }

    function getIndicatorContent(status) {
        if (status === 'done') return checkSVG;
        if (status === 'running') return dotSVG;
        if (status === 'error') return crossSVG;
        return '';
    }

    function updateStage(data) {
        var el = document.getElementById('stage-' + data.index);
        if (!el) return;

        // Mark ALL previous stages as done (handles skipped/undetected stages)
        for (var i = 0; i < data.index; i++) {
            var prev = document.getElementById('stage-' + i);
            if (prev && !prev.classList.contains('stage-done') && !prev.classList.contains('stage-error')) {
                prev.className = 'stage-item stage-done';
                var prevIndicator = prev.querySelector('.stage-indicator');
                if (prevIndicator) prevIndicator.innerHTML = checkSVG;
            }
        }

        el.className = 'stage-item stage-' + data.status;
        var indicator = el.querySelector('.stage-indicator');
        if (indicator) indicator.innerHTML = getIndicatorContent(data.status);
        var nameEl = el.querySelector('.stage-name');
        if (nameEl) nameEl.textContent = data.name;
        stageCounter.textContent = (data.index + 1) + ' / ' + data.total;

        el.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }

    function updateAllStages(stages) {
        var doneCount = 0;
        for (var i = 0; i < stages.length; i++) {
            var el = document.getElementById('stage-' + i);
            if (!el) continue;
            el.className = 'stage-item stage-' + stages[i].status;
            var indicator = el.querySelector('.stage-indicator');
            if (indicator) indicator.innerHTML = getIndicatorContent(stages[i].status);
            var nameEl = el.querySelector('.stage-name');
            if (nameEl) nameEl.textContent = stages[i].name;
            if (stages[i].status === 'done') doneCount++;
        }
        stageCounter.textContent = doneCount + ' / ' + stages.length;
    }

    // --- SSE connection (with auth token as query param) ---

    function connectSSE(deploymentId) {
        var evtSource = new EventSource('/api/deployments/' + deploymentId + '/stream?token=' + encodeURIComponent(authToken));

        evtSource.addEventListener('stages', function(e) {
            var stages = JSON.parse(e.data);
            updateAllStages(stages);
        });

        evtSource.addEventListener('log', function(e) {
            rawLogLines.push(e.data);
            appendLog(e.data);
        });

        evtSource.addEventListener('stage', function(e) {
            var data = JSON.parse(e.data);
            updateStage(data);
            // Insert section header in log panel
            var header = document.createElement('span');
            header.className = 'log-line phase-header';
            header.textContent = data.name + '\n';
            logOutput.appendChild(header);
            var container = document.getElementById('log-container');
            container.scrollTop = container.scrollHeight;
        });

        evtSource.addEventListener('done', function(e) {
            var data = JSON.parse(e.data);
            updateFinalStatus(data.status);
            if (data.stages) {
                updateAllStages(data.stages);
            }
            evtSource.close();
        });

        evtSource.onerror = function() {
            evtSource.close();
            pollStatus(deploymentId);
        };
    }

    function updateFinalStatus(status) {
        // Hide live dot
        if (liveDot) liveDot.style.display = 'none';

        if (status === 'success') {
            statusBadge.className = 'badge badge-success';
            statusBadge.textContent = 'Success';
            showResultPanel('success');
            retryBtn.classList.add('hidden');
        } else if (status === 'failed') {
            statusBadge.className = 'badge badge-failed';
            statusBadge.textContent = 'Failed';
            showResultPanel('failed');
            if (lastPayload) retryBtn.classList.remove('hidden');
        }
        newDeployBtn.classList.remove('hidden');
    }

    // Parse deployment info from raw log lines
    function parseDeployInfo() {
        var info = { httpPort: '', httpsPort: '', csURL: '', csUser: '', csPass: '', csAPIKey: '', csSecretKey: '' };
        for (var i = 0; i < rawLogLines.length; i++) {
            var line = rawLogLines[i];
            var httpMatch = line.match(/External Port 80\s+->\s+Internal\s+\S+:(\d+)/);
            if (httpMatch) info.httpPort = httpMatch[1];
            var httpsMatch = line.match(/External Port 443\s+->\s+Internal\s+\S+:(\d+)/);
            if (httpsMatch) info.httpsPort = httpsMatch[1];
            var csURLMatch = line.match(/URL:\s+(http:\/\/\S+:8080\S*)/);
            if (csURLMatch) info.csURL = csURLMatch[1];
            if (line.match(/STACKBILL CLOUDSTACK USER/)) {
                // Next few lines have Username, Password, API Key, Secret Key
                for (var j = i + 1; j < Math.min(i + 6, rawLogLines.length); j++) {
                    var uMatch = rawLogLines[j].match(/Username:\s+(\S+)/);
                    if (uMatch) info.csUser = uMatch[1];
                    var pMatch = rawLogLines[j].match(/Password:\s+(\S+)/);
                    if (pMatch) info.csPass = pMatch[1];
                    var akMatch = rawLogLines[j].match(/API Key:\s+(\S+)/);
                    if (akMatch) info.csAPIKey = akMatch[1];
                    var skMatch = rawLogLines[j].match(/Secret Key:\s+(\S+)/);
                    if (skMatch) info.csSecretKey = skMatch[1];
                }
            }
        }
        return info;
    }

    // XSS-safe result panel: all user-derived data is escaped before insertion
    function showResultPanel(status) {
        var oldResult = document.getElementById('result-panel');
        if (oldResult) oldResult.remove();

        var panel = document.createElement('div');
        panel.id = 'result-panel';
        panel.className = 'result-panel result-' + status;

        var safeDomain = escapeHtml(currentDomain);
        var safeIP = escapeHtml(currentServerIP);
        var safeId = escapeHtml(currentDeploymentId);
        var logDownloadURL = '/api/deployments/' + encodeURIComponent(currentDeploymentId) + '/log?token=' + encodeURIComponent(authToken);

        if (status === 'success') {
            var portalURL = 'https://' + safeDomain + '/admin';
            var deployInfo = parseDeployInfo();
            var isSimulator = lastPayload && lastPayload.cloudstack_mode === 'simulator';

            var html =
                '<h3>' + checkSVGDark + ' Deployment Complete</h3>' +
                '<div class="result-grid">' +
                    '<div class="result-row">' +
                        '<span class="result-label">Portal URL</span>' +
                        '<a href="' + portalURL + '" target="_blank" rel="noopener noreferrer" class="result-link">' + portalURL + '</a>' +
                    '</div>' +
                    '<div class="result-row">' +
                        '<span class="result-label">Server</span>' +
                        '<span class="result-value">' + safeIP + '</span>' +
                    '</div>' +
                    '<div class="result-row">' +
                        '<span class="result-label">Credentials</span>' +
                        '<code class="result-path">/root/stackbill-credentials.txt</code>' +
                    '</div>' +
                    '<div class="result-row">' +
                        '<span class="result-label">Deploy Log</span>' +
                        '<a href="' + escapeHtml(logDownloadURL) + '" class="result-download" download>Download Full Log</a>' +
                    '</div>' +
                '</div>';

            // CloudStack user section
            if (isSimulator && deployInfo.csUser) {
                html += '<div class="result-section">' +
                    '<h4>CloudStack User</h4>' +
                    '<div class="result-grid">' +
                        '<div class="result-row">' +
                            '<span class="result-label">Username</span>' +
                            '<span class="result-value">' + escapeHtml(deployInfo.csUser) + '</span>' +
                        '</div>' +
                        '<div class="result-row">' +
                            '<span class="result-label">Password</span>' +
                            '<span class="result-value">' + escapeHtml(deployInfo.csPass) + '</span>' +
                        '</div>';
                if (deployInfo.csAPIKey) {
                    html += '<div class="result-row">' +
                            '<span class="result-label">API Key</span>' +
                            '<span class="result-value result-mono">' + escapeHtml(deployInfo.csAPIKey) + '</span>' +
                        '</div>' +
                        '<div class="result-row">' +
                            '<span class="result-label">Secret Key</span>' +
                            '<span class="result-value result-mono">' + escapeHtml(deployInfo.csSecretKey) + '</span>' +
                        '</div>';
                }
                html += '</div></div>';
            }

            // Firewall / NAT rules section
            if (deployInfo.httpPort || deployInfo.httpsPort) {
                html += '<div class="result-section">' +
                    '<h4>Firewall / NAT / Load Balancer Rules</h4>' +
                    '<div class="result-rules">' +
                        '<div class="result-rule">' +
                            '<span>External Port <strong>80</strong></span>' +
                            '<span class="rule-arrow">&rarr;</span>' +
                            '<span>Internal ' + safeIP + ':' + escapeHtml(deployInfo.httpPort) + ' <em>(HTTP)</em></span>' +
                        '</div>' +
                        '<div class="result-rule">' +
                            '<span>External Port <strong>443</strong></span>' +
                            '<span class="rule-arrow">&rarr;</span>' +
                            '<span>Internal ' + safeIP + ':' + escapeHtml(deployInfo.httpsPort) + ' <em>(HTTPS)</em></span>' +
                        '</div>';
                if (isSimulator) {
                    html += '<div class="result-rule">' +
                            '<span>External Port <strong>8080</strong></span>' +
                            '<span class="rule-arrow">&rarr;</span>' +
                            '<span>Internal ' + safeIP + ':8080 <em>(CloudStack)</em></span>' +
                        '</div>';
                }
                html += '</div></div>';
            }

            html += '<p class="result-hint">SSH into <strong>' + safeIP + '</strong> to view MySQL, MongoDB, and RabbitMQ passwords.</p>';
            panel.innerHTML = html;
        } else {
            var errorLines = [];
            var allLines = logOutput.querySelectorAll('.log-line.error');
            allLines.forEach(function(el) { errorLines.push(el.textContent.trim()); });
            var lastError = errorLines.length > 0 ? errorLines[errorLines.length - 1] : 'Unknown error';

            panel.innerHTML =
                '<h3>' + crossSVG + ' Deployment Failed</h3>' +
                '<div class="result-grid">' +
                    '<div class="result-row">' +
                        '<span class="result-label">Error</span>' +
                        '<span class="result-value result-error-text">' + escapeHtml(lastError) + '</span>' +
                    '</div>' +
                    '<div class="result-row">' +
                        '<span class="result-label">Server</span>' +
                        '<span class="result-value">' + safeIP + '</span>' +
                    '</div>' +
                    '<div class="result-row">' +
                        '<span class="result-label">Deploy Log</span>' +
                        '<a href="' + escapeHtml(logDownloadURL) + '" class="result-download" download>Download Full Log</a>' +
                    '</div>' +
                '</div>' +
                '<p class="result-hint">Check the logs above for details. SSH into <strong>' + safeIP + '</strong> to investigate.</p>';
        }

        var actions = document.querySelector('.dashboard-actions');
        actions.parentNode.insertBefore(panel, actions);
    }

    // --- Log display ---

    function stripAnsi(str) {
        return str.replace(/\x1B\[[0-9;]*[a-zA-Z]/g, '')
                  .replace(/\x1B\].*?(\x07|\x1B\\)/g, '')
                  .replace(/[\x00-\x09\x0B\x0C\x0E-\x1F]/g, '');
    }

    function isImportantLine(line) {
        if (line.match(/^\[INFO\]|^\[WARN\]|^\[ERROR\]/)) return true;
        if (line.match(/ERROR|FATAL|FAIL|Could not|Unable to|Permission denied/i)) return true;
        if (line.match(/WARNING|warn:/i)) return true;
        return false;
    }

    function appendLog(line) {
        var clean = stripAnsi(line);
        if (clean.trim() === '') return;
        if (clean.match(/^[═╔╗╚╝║─┌┐└┘│\-\+]+$/)) return;
        if (clean.match(/^[║|]\s.*[║|]$/)) return;
        if (!isImportantLine(clean)) return;

        var span = document.createElement('span');
        span.className = 'log-line';

        if (clean.match(/ERROR|FATAL|FAIL|Could not|Unable to|Permission denied/i)) {
            span.classList.add('error');
        } else if (clean.match(/\[WARN\]|WARNING|warn:/i)) {
            span.classList.add('warning');
        } else if (clean.match(/\[INFO\]|completed|successfully|done|ready|installed|✔|condition met|generated|passed|validated/i)) {
            span.classList.add('success');
        }

        span.textContent = clean + '\n';
        logOutput.appendChild(span);

        var container = document.getElementById('log-container');
        container.scrollTop = container.scrollHeight;
    }

    // --- Polling fallback (with auth) ---

    function pollStatus(deploymentId) {
        var interval = setInterval(async function() {
            try {
                var response = await fetch('/api/deployments/' + deploymentId, {
                    headers: { 'Authorization': 'Bearer ' + authToken }
                });

                if (response.status === 401) {
                    clearInterval(interval);
                    handleAuthFailure();
                    return;
                }

                var data = await response.json();

                if (data.stages) {
                    updateAllStages(data.stages);
                }

                logOutput.innerHTML = '';
                if (data.logs) {
                    data.logs.forEach(function(line) {
                        appendLog(line);
                    });
                }

                if (data.status === 'success' || data.status === 'failed') {
                    clearInterval(interval);
                    updateFinalStatus(data.status);
                }
            } catch (err) {
                console.error('Poll failed:', err);
            }
        }, 3000);
    }

    // --- Utilities ---

    function escapeHtml(str) {
        var div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    function initFloatingLabels() {
        var inputs = document.querySelectorAll('.form-group input');
        inputs.forEach(function(input) {
            var group = input.parentElement;
            function update() {
                if (input.value && input.value !== '') {
                    group.classList.add('filled');
                } else {
                    group.classList.remove('filled');
                }
            }
            update();
            input.addEventListener('input', update);
            input.addEventListener('change', update);
            input.addEventListener('focus', function() { group.classList.add('focused'); });
            input.addEventListener('blur', function() {
                group.classList.remove('focused');
                update();
            });
        });
    }

    // --- Retry ---

    window.retryDeployment = async function() {
        if (!lastPayload) return;

        retryBtn.disabled = true;
        retryBtn.textContent = 'Retrying...';

        // Remove result panel
        var oldResult = document.getElementById('result-panel');
        if (oldResult) oldResult.remove();

        try {
            var response = await fetch('/api/deploy', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': 'Bearer ' + authToken
                },
                body: JSON.stringify(lastPayload)
            });

            if (!response.ok) {
                if (response.status === 401) {
                    handleAuthFailure();
                    return;
                }
                var err = await response.json();
                throw new Error(err.error || 'Retry failed to start');
            }

            var data = await response.json();
            showDashboard(data.id, data.stages);
        } catch (err) {
            alert('Retry failed: ' + err.message);
        }

        retryBtn.disabled = false;
        retryBtn.textContent = 'Retry Deployment';
    };

    // --- Reset ---

    window.resetForm = function() {
        formSection.classList.remove('hidden');
        dashboardSection.classList.add('hidden');
        appContainer.classList.remove('container-wide');
        newDeployBtn.classList.add('hidden');
        retryBtn.classList.add('hidden');
        deployBtn.disabled = true;
        deployBtn.textContent = 'Deploy StackBill';
        form.reset();
        document.getElementById('cloudstack_version').value = '4.21.0.0';
        document.getElementById('ssl_letsencrypt').checked = true;
        sslLetsencryptOptions.classList.remove('hidden');
        sslCustomOptions.classList.add('hidden');
        document.getElementById('cs_existing').checked = true;
        csSimulatorOptions.classList.add('hidden');
        lastPayload = null;
        initFloatingLabels();
    };
});
