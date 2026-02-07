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

    // Handle form submission
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

        deployBtn.disabled = true;
        deployBtn.innerHTML = '<span class="btn-deploying"><span class="spinner"></span> Deploying...</span>';

        try {
            var response = await fetch('/api/deploy', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });

            if (!response.ok) {
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

    function showDashboard(deploymentId, stages) {
        currentDomain = document.getElementById('domain').value;
        currentServerIP = document.getElementById('server_ip').value;
        currentDeploymentId = deploymentId;
        formSection.classList.add('hidden');
        dashboardSection.classList.remove('hidden');
        appContainer.classList.add('container-wide');
        logOutput.innerHTML = '';
        statusBadge.className = 'badge badge-running';
        statusBadge.textContent = 'Running';
        newDeployBtn.classList.add('hidden');
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
        stageCounter.textContent = data.done_count + ' / ' + data.total;

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

    // --- SSE connection ---

    function connectSSE(deploymentId) {
        var evtSource = new EventSource('/api/deployments/' + deploymentId + '/stream');

        evtSource.addEventListener('stages', function(e) {
            var stages = JSON.parse(e.data);
            updateAllStages(stages);
        });

        evtSource.addEventListener('log', function(e) {
            appendLog(e.data);
        });

        evtSource.addEventListener('stage', function(e) {
            var data = JSON.parse(e.data);
            updateStage(data);
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
        } else if (status === 'failed') {
            statusBadge.className = 'badge badge-failed';
            statusBadge.textContent = 'Failed';
            showResultPanel('failed');
        }
        newDeployBtn.classList.remove('hidden');
    }

    function showResultPanel(status) {
        var oldResult = document.getElementById('result-panel');
        if (oldResult) oldResult.remove();

        var panel = document.createElement('div');
        panel.id = 'result-panel';
        panel.className = 'result-panel result-' + status;

        var logDownloadURL = '/api/deployments/' + currentDeploymentId + '/log';

        if (status === 'success') {
            var portalURL = 'https://' + currentDomain + '/admin';
            panel.innerHTML =
                '<h3>' + checkSVGDark + ' Deployment Complete</h3>' +
                '<div class="result-grid">' +
                    '<div class="result-row">' +
                        '<span class="result-label">Portal URL</span>' +
                        '<a href="' + portalURL + '" target="_blank" class="result-link">' + portalURL + '</a>' +
                    '</div>' +
                    '<div class="result-row">' +
                        '<span class="result-label">Server</span>' +
                        '<span class="result-value">' + currentServerIP + '</span>' +
                    '</div>' +
                    '<div class="result-row">' +
                        '<span class="result-label">Credentials</span>' +
                        '<code class="result-path">/root/stackbill-credentials.txt</code>' +
                    '</div>' +
                    '<div class="result-row">' +
                        '<span class="result-label">Deploy Log</span>' +
                        '<a href="' + logDownloadURL + '" class="result-download" download>Download Full Log</a>' +
                    '</div>' +
                '</div>' +
                '<p class="result-hint">SSH into <strong>' + currentServerIP + '</strong> to view MySQL, MongoDB, and RabbitMQ passwords.</p>';
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
                        '<span class="result-value">' + currentServerIP + '</span>' +
                    '</div>' +
                    '<div class="result-row">' +
                        '<span class="result-label">Deploy Log</span>' +
                        '<a href="' + logDownloadURL + '" class="result-download" download>Download Full Log</a>' +
                    '</div>' +
                '</div>' +
                '<p class="result-hint">Check the logs above for details. SSH into <strong>' + currentServerIP + '</strong> to investigate.</p>';
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

    // Only show important lines — phase headers, [INFO], [WARN], [ERROR], key status
    // Verbose apt/dpkg/package output is filtered out (saved on server via tee)
    function isImportantLine(line) {
        // Phase headers (log_step output from script)
        if (line.match(/^\s{0,4}(Checking |Installing |Setting up |Deploying |Generating |Waiting |Configuring |Creating |Saving |STACKBILL)/)) return true;
        // Tagged lines from script
        if (line.match(/^\[INFO\]|^\[WARN\]|^\[ERROR\]/)) return true;
        // Connection/deployer messages
        if (line.match(/^Connecting to |^Connected |^Uploading |^Script uploaded|^Starting deployment|^Deployment completed/)) return true;
        // Errors that should always show
        if (line.match(/ERROR|FATAL|FAIL|Could not|Unable to|Permission denied/i)) return true;
        // Warnings
        if (line.match(/WARNING|warn:/i)) return true;
        // Configuration summary block
        if (line.match(/Configuration Summary|Domain:|SSL Mode:|Email:|CloudStack:|ECR Token:/)) return true;
        // Script banner lines
        if (line.match(/This script will install:|You will be prompted for:/)) return true;
        // Key completion messages
        if (line.match(/already installed|already running|condition met|successfully|completed|generated|passed|validated/i)) return true;
        // Server info
        if (line.match(/Server IP:|New passwords generated/)) return true;

        // Everything else is verbose noise — filtered out
        return false;
    }

    function appendLog(line) {
        var clean = stripAnsi(line);
        if (clean.trim() === '') return;
        // Skip decorative border lines
        if (clean.match(/^[═╔╗╚╝║─┌┐└┘│\-\+]+$/)) return;
        // Skip box-drawing content lines (║ ... ║)
        if (clean.match(/^[║|]\s.*[║|]$/)) return;

        // Filter: only show important lines
        if (!isImportantLine(clean)) return;

        var span = document.createElement('span');
        span.className = 'log-line';

        // Classify the line
        if (clean.match(/^\s{0,4}(Checking |Installing |Setting up |Deploying |Generating |Waiting |Configuring |Creating |Saving |STACKBILL)/)) {
            span.classList.add('phase-header');
        } else if (clean.match(/ERROR|FATAL|FAIL|Could not|Unable to|Permission denied/i)) {
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

    // --- Polling fallback ---

    function pollStatus(deploymentId) {
        var interval = setInterval(async function() {
            try {
                var response = await fetch('/api/deployments/' + deploymentId);
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
            // Set initial state
            if (input.value && input.value !== '') {
                input.classList.add('has-value');
            }
            // Listen for changes
            input.addEventListener('input', function() {
                if (this.value && this.value !== '') {
                    this.classList.add('has-value');
                } else {
                    this.classList.remove('has-value');
                }
            });
            input.addEventListener('change', function() {
                if (this.value && this.value !== '') {
                    this.classList.add('has-value');
                } else {
                    this.classList.remove('has-value');
                }
            });
        });
    }

    // --- Reset ---

    window.resetForm = function() {
        formSection.classList.remove('hidden');
        dashboardSection.classList.add('hidden');
        appContainer.classList.remove('container-wide');
        newDeployBtn.classList.add('hidden');
        deployBtn.disabled = false;
        deployBtn.textContent = 'Deploy StackBill';
        form.reset();
        document.getElementById('ssh_port').value = '22';
        document.getElementById('cloudstack_version').value = '4.21.0.0';
        document.getElementById('ssl_letsencrypt').checked = true;
        sslLetsencryptOptions.classList.remove('hidden');
        sslCustomOptions.classList.add('hidden');
        document.getElementById('cs_existing').checked = true;
        csSimulatorOptions.classList.add('hidden');
        initFloatingLabels();
    };
});
