document.addEventListener('DOMContentLoaded', function() {
    const form = document.getElementById('deploy-form');
    const formSection = document.getElementById('deploy-form-section');
    const progressSection = document.getElementById('progress-section');
    const logOutput = document.getElementById('log-output');
    const statusBadge = document.getElementById('status-badge');
    const deployBtn = document.getElementById('deploy-btn');
    const newDeployBtn = document.getElementById('new-deploy-btn');
    const cloudstackCheckbox = document.getElementById('enable_cloudstack');
    const cloudstackOptions = document.getElementById('cloudstack-options');

    // Toggle CloudStack options
    cloudstackCheckbox.addEventListener('change', function() {
        cloudstackOptions.classList.toggle('hidden', !this.checked);
    });

    // Handle form submission
    form.addEventListener('submit', async function(e) {
        e.preventDefault();

        const payload = {
            server_ip: document.getElementById('server_ip').value,
            ssh_user: document.getElementById('ssh_user').value,
            ssh_pass: document.getElementById('ssh_pass').value,
            ssh_port: parseInt(document.getElementById('ssh_port').value) || 22,
            domain: document.getElementById('domain').value,
            enable_ssl: document.getElementById('enable_ssl').checked,
            enable_monitoring: document.getElementById('enable_monitoring').checked,
            enable_cloudstack: cloudstackCheckbox.checked,
            cloudstack_version: document.getElementById('cloudstack_version').value
        };

        deployBtn.disabled = true;
        deployBtn.textContent = 'Starting deployment...';

        try {
            const response = await fetch('/api/deploy', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });

            if (!response.ok) {
                const err = await response.json();
                throw new Error(err.error || 'Deployment failed to start');
            }

            const data = await response.json();
            showProgress(data.id);
        } catch (err) {
            alert('Error: ' + err.message);
            deployBtn.disabled = false;
            deployBtn.textContent = 'Deploy StackBill';
        }
    });

    function showProgress(deploymentId) {
        formSection.classList.add('hidden');
        progressSection.classList.remove('hidden');
        logOutput.innerHTML = '';
        statusBadge.className = 'badge badge-running';
        statusBadge.textContent = 'Running';

        connectWebSocket(deploymentId);
    }

    function connectWebSocket(deploymentId) {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const ws = new WebSocket(protocol + '//' + window.location.host + '/ws/logs/' + deploymentId);

        ws.onmessage = function(event) {
            const data = JSON.parse(event.data);
            appendLog(data.log);
        };

        ws.onclose = function() {
            // Check final status
            checkDeploymentStatus(deploymentId);
        };

        ws.onerror = function() {
            appendLog('WebSocket connection error. Falling back to polling...');
            pollLogs(deploymentId);
        };
    }

    function appendLog(line) {
        const span = document.createElement('span');
        span.className = 'log-line';

        if (line.startsWith('ERROR')) {
            span.classList.add('error');
        } else if (line.includes('completed') || line.includes('successfully')) {
            span.classList.add('success');
        } else if (line.startsWith('==>') || line.startsWith('Step')) {
            span.classList.add('step');
        }

        span.textContent = line + '\n';
        logOutput.appendChild(span);

        // Auto-scroll to bottom
        const container = document.getElementById('log-container');
        container.scrollTop = container.scrollHeight;
    }

    async function checkDeploymentStatus(deploymentId) {
        try {
            const response = await fetch('/api/deployments/' + deploymentId);
            const data = await response.json();

            if (data.status === 'success') {
                statusBadge.className = 'badge badge-success';
                statusBadge.textContent = 'Success';
            } else if (data.status === 'failed') {
                statusBadge.className = 'badge badge-failed';
                statusBadge.textContent = 'Failed';
            }

            newDeployBtn.classList.remove('hidden');
        } catch (err) {
            console.error('Status check failed:', err);
        }
    }

    function pollLogs(deploymentId) {
        const interval = setInterval(async function() {
            try {
                const response = await fetch('/api/deployments/' + deploymentId);
                const data = await response.json();

                // Clear and re-render all logs
                logOutput.innerHTML = '';
                if (data.logs) {
                    data.logs.forEach(function(line) {
                        appendLog(line);
                    });
                }

                if (data.status === 'success' || data.status === 'failed') {
                    clearInterval(interval);
                    if (data.status === 'success') {
                        statusBadge.className = 'badge badge-success';
                        statusBadge.textContent = 'Success';
                    } else {
                        statusBadge.className = 'badge badge-failed';
                        statusBadge.textContent = 'Failed';
                    }
                    newDeployBtn.classList.remove('hidden');
                }
            } catch (err) {
                console.error('Poll failed:', err);
            }
        }, 3000);
    }

    // Reset form for new deployment
    window.resetForm = function() {
        formSection.classList.remove('hidden');
        progressSection.classList.add('hidden');
        newDeployBtn.classList.add('hidden');
        deployBtn.disabled = false;
        deployBtn.textContent = 'Deploy StackBill';
        form.reset();
        document.getElementById('ssh_port').value = '22';
        document.getElementById('cloudstack_version').value = '4.21.0.0';
        cloudstackOptions.classList.add('hidden');
    };
});
