"""
Custom Ansible stdout callback plugin for StackBill Deployer.

Formats Ansible output to match the log format expected by the
StackBill frontend (app.js) and Go backend (detectStage).

Output format:
  - Stage headers: bordered lines with ║ characters for frontend phase-header detection
  - Task results: [INFO], [WARN], [ERROR] prefixed lines
  - Debug messages: forwarded as [INFO] lines (used for credential/summary output)
"""

from __future__ import absolute_import, division, print_function

__metaclass__ = type

import sys
import json

from ansible.plugins.callback import CallbackBase


# Maps Ansible role names to the stage header text that must match
# the MatchKey (or Name fallback) in models.BuildStages()
ROLE_STAGE_MAP = {
    "check_requirements": "Checking System Requirements",
    "k3s": "Installing K3s",
    "helm": "Installing Helm",
    "istio": "Installing Istio",
    "certbot": "Installing Certbot",
    "letsencrypt_cert": "Generating Let's Encrypt SSL Certificate",
    "certificate_renewal": "Setting up Automatic Certificate Renewal",
    "mariadb": "Installing MariaDB",
    "mongodb": "Installing MongoDB",
    "rabbitmq": "Installing RabbitMQ",
    "nfs": "Setting up NFS Storage",
    "k8s_namespace": "Setting up Kubernetes Namespace",
    "ecr_credentials": "Setting up AWS ECR Credentials",
    "tls_secret": "Setting up TLS Secret",
    "deploy_stackbill": "Deploying StackBill from ECR",
    "istio_gateway": "Setting up Istio Gateway",
    "wait_for_pods": "Waiting for StackBill Pods",
    "podman": "Installing Podman",
    "cloudstack_simulator": "Deploying CloudStack Simulator",
    "cloudstack_rabbitmq": "Configuring CloudStack RabbitMQ",
    "cloudstack_user": "Creating CloudStack Admin User for StackBill",
    "save_credentials": "Saving Credentials",
}

BORDER = "═" * 64


def _emit(msg):
    """Write a line to stdout and flush immediately for real-time streaming."""
    sys.stdout.write(msg + "\n")
    sys.stdout.flush()


def _get_role_name(task):
    """Extract the role name from a task, if any."""
    if task._role:
        return task._role._role_name
    return None


class CallbackModule(CallbackBase):
    CALLBACK_VERSION = 2.0
    CALLBACK_TYPE = "stdout"
    CALLBACK_NAME = "stackbill_log"

    def __init__(self):
        super(CallbackModule, self).__init__()
        self._current_role = None

    def _emit_stage_header(self, role_name):
        """Emit bordered stage header matching frontend regex and backend detectStage."""
        stage_name = ROLE_STAGE_MAP.get(role_name)
        if not stage_name:
            return
        # Border line (filtered by frontend line 543: pure box-drawing chars)
        _emit(BORDER)
        # Stage name line (matched by frontend stageMatch regex and backend detectStage)
        _emit("║  {}  ║".format(stage_name))
        _emit(BORDER)

    def _check_role_change(self, task):
        """Detect role transitions and emit stage headers."""
        role_name = _get_role_name(task)
        if role_name and role_name != self._current_role:
            self._current_role = role_name
            self._emit_stage_header(role_name)

    # -- Playbook events (suppressed to avoid Ansible noise) --

    def v2_playbook_on_play_start(self, play):
        pass

    def v2_playbook_on_task_start(self, task, is_conditional):
        self._check_role_change(task)

    def v2_playbook_on_handler_task_start(self, task):
        self._check_role_change(task)

    # -- Runner events --

    def v2_runner_on_ok(self, result, **kwargs):
        task_name = result._task.get_name()

        # Forward debug messages as [INFO] lines
        if result._task.action in ("ansible.builtin.debug", "debug"):
            msg = result._result.get("msg", "")
            if isinstance(msg, list):
                for line in msg:
                    _emit("[INFO] {}".format(line))
            elif msg:
                # Multi-line debug messages (used for credential output)
                for line in str(msg).split("\n"):
                    if line.strip():
                        _emit("[INFO] {}".format(line))
            return

        # Forward assert success messages (e.g. "OS: Ubuntu 22.04 ✓")
        if result._task.action in ("ansible.builtin.assert", "assert"):
            msg = result._result.get("msg", "")
            if msg and msg != "All assertions passed":
                _emit("[INFO] {}".format(msg))
            else:
                _emit("[INFO] {} ... ok".format(task_name))
            return

        # Skip gather_facts noise
        if task_name.lower() in ("gathering facts", "gather facts", "setup"):
            return

        if result.is_changed():
            _emit("[INFO] {} ... changed".format(task_name))
        else:
            _emit("[INFO] {} ... ok".format(task_name))

    def v2_runner_on_failed(self, result, ignore_errors=False, **kwargs):
        task_name = result._task.get_name()
        msg = result._result.get("msg", "")
        stderr = result._result.get("stderr", "")
        error_detail = msg or stderr or "unknown error"

        if ignore_errors:
            _emit("[WARN] {} ... FAILED (ignored): {}".format(task_name, error_detail))
        else:
            _emit("[ERROR] {} ... FAILED: {}".format(task_name, error_detail))

    def v2_runner_on_skipped(self, result, **kwargs):
        # Silent — don't emit anything for skipped tasks
        pass

    def v2_runner_on_unreachable(self, result, **kwargs):
        msg = result._result.get("msg", "")
        _emit("[ERROR] Target unreachable: {}".format(msg))

    # -- Stats (suppressed) --

    def v2_playbook_on_stats(self, stats):
        # Emit final summary
        hosts = sorted(stats.processed.keys())
        for h in hosts:
            s = stats.summarize(h)
            if s["failures"] > 0 or s["unreachable"] > 0:
                _emit("[ERROR] Deployment failed: {} failures, {} unreachable".format(
                    s["failures"], s["unreachable"]))
            else:
                _emit("[INFO] Deployment completed: {} ok, {} changed".format(
                    s["ok"], s["changed"]))
