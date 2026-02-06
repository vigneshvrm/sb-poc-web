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

    // SSL mode toggle
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

    // CloudStack mode toggle
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
        deployBtn.textContent = 'Starting deployment...';

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

    function showDashboard(deploymentId, stages) {
        currentDomain = document.getElementById('domain').value;
        currentServerIP = document.getElementById('server_ip').value;
        formSection.classList.add('hidden');
        dashboardSection.classList.remove('hidden');
        appContainer.classList.add('container-wide');
        logOutput.innerHTML = '';
        statusBadge.className = 'badge badge-running';
        statusBadge.textContent = 'Running';
        newDeployBtn.classList.add('hidden');
        // Clear any previous result
        var oldResult = document.getElementById('result-panel');
        if (oldResult) oldResult.remove();

        renderStages(stages);
        connectSSE(deploymentId);
    }

    // --- Stage rendering ---

    function renderStages(stages) {
        stageList.innerHTML = '';
        for (var i = 0; i < stages.length; i++) {
            var stage = stages[i];
            var div = document.createElement('div');
            div.className = 'stage-item stage-' + stage.status;
            div.id = 'stage-' + i;
            div.textContent = stageIcon(stage.status) + ' ' + stage.name;
            stageList.appendChild(div);
        }
        stageCounter.textContent = '0 / ' + stages.length;
    }

    function stageIcon(status) {
        if (status === 'done') return '\u2713';
        if (status === 'running') return '\u25CF';
        if (status === 'error') return '\u2717';
        return '\u25CB';
    }

    function updateStage(data) {
        var el = document.getElementById('stage-' + data.index);
        if (!el) return;

        // Mark previous stages as done visually
        for (var i = 0; i < data.index; i++) {
            var prev = document.getElementById('stage-' + i);
            if (prev && prev.classList.contains('stage-running')) {
                prev.className = 'stage-item stage-done';
                prev.textContent = '\u2713 ' + prev.textContent.substring(2);
            }
        }

        el.className = 'stage-item stage-' + data.status;
        el.textContent = stageIcon(data.status) + ' ' + data.name;
        stageCounter.textContent = data.done_count + ' / ' + data.total;

        el.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }

    function updateAllStages(stages) {
        var doneCount = 0;
        for (var i = 0; i < stages.length; i++) {
            var el = document.getElementById('stage-' + i);
            if (!el) continue;
            el.className = 'stage-item stage-' + stages[i].status;
            el.textContent = stageIcon(stages[i].status) + ' ' + stages[i].name;
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

        if (status === 'success') {
            panel.innerHTML =
                '<h3>Deployment Complete</h3>' +
                '<div class="result-details">' +
                '<strong>Portal URL:</strong> <a href="https://' + currentDomain + '/admin" target="_blank">https://' + currentDomain + '/admin</a><br>' +
                '<strong>Server:</strong> ' + currentServerIP + '<br><br>' +
                'Credentials have been saved to <strong>/root/stackbill-credentials.txt</strong> on the server.<br>' +
                'SSH into the server to view MySQL, MongoDB, and RabbitMQ passwords.' +
                '</div>';
        } else {
            // Find the last error line from logs
            var errorLines = [];
            var allLines = logOutput.querySelectorAll('.log-line.error');
            allLines.forEach(function(el) { errorLines.push(el.textContent.trim()); });
            var lastError = errorLines.length > 0 ? errorLines[errorLines.length - 1] : 'Unknown error';

            panel.innerHTML =
                '<h3>Deployment Failed</h3>' +
                '<div class="result-details">' +
                '<strong>Error:</strong> ' + lastError + '<br><br>' +
                'Check the logs above for details. You can SSH into <strong>' + currentServerIP + '</strong> to investigate.<br>' +
                'Deployment log saved on the server.' +
                '</div>';
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

    function appendLog(line) {
        var clean = stripAnsi(line);
        if (clean.trim() === '') return;
        // Skip decorative border lines
        if (clean.match(/^[═╔╗╚╝║─]+$/)) return;

        var span = document.createElement('span');
        span.className = 'log-line';

        // Phase header — log_step titles from the script
        if (clean.match(/^\s{0,4}(Checking |Installing |Setting up |Deploying |Generating |Waiting |Configuring |Creating |Saving |STACKBILL)/)) {
            span.classList.add('phase-header');
        } else if (clean.match(/ERROR|E:|FATAL|FAIL|Could not|Unable to|Permission denied/i)) {
            span.classList.add('error');
        } else if (clean.match(/\[WARN\]|WARNING|warn:/i)) {
            span.classList.add('warning');
        } else if (clean.match(/\[INFO\]|completed|successfully|done|ready|installed|✔|condition met/i)) {
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
    };
});
