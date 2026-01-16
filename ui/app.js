const API_BASE_URL = window.location.origin + '/api';

// Set default datetime to 5 minutes from now
function setDefaultDateTime() {
    const now = new Date();
    now.setMinutes(now.getMinutes() + 5);
    const localDateTime = now.toISOString().slice(0, 16);
    document.getElementById('sendAt').value = localDateTime;
}

// Format date for display
function formatDate(dateString) {
    const date = new Date(dateString);
    return date.toLocaleString();
}

// Get status badge HTML
function getStatusBadge(status) {
    const statusMap = {
        'pending': { class: 'status-pending', text: 'Pending' },
        'sent': { class: 'status-sent', text: 'Sent' },
        'failed': { class: 'status-failed', text: 'Failed' },
        'cancelled': { class: 'status-cancelled', text: 'Cancelled' },
        'retrying': { class: 'status-retrying', text: 'Retrying' }
    };

    const statusInfo = statusMap[status] || { class: 'status-pending', text: status };
    return `<span class="badge ${statusInfo.class}">${statusInfo.text}</span>`;
}

// Create notification
async function createNotification(event) {
    event.preventDefault();

    const message = document.getElementById('message').value;
    const sendAt = document.getElementById('sendAt').value;
    const maxRetries = document.getElementById('maxRetries').value || 3;

    const notification = {
        message: message,
        send_at: new Date(sendAt).toISOString(),
        max_retries: parseInt(maxRetries)
    };

    try {
        const response = await fetch(`${API_BASE_URL}/notify`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(notification)
        });

        if (response.ok) {
            alert('Notification scheduled successfully!');
            document.getElementById('notificationForm').reset();
            setDefaultDateTime();
            loadNotifications();
            loadStats();
        } else {
            const error = await response.text();
            alert(`Error: ${error}`);
        }
    } catch (error) {
        alert(`Network error: ${error.message}`);
    }
}

// Load all notifications
async function loadNotifications() {
    try {
        const response = await fetch(`${API_BASE_URL}/notify`);
        const notifications = await response.json();

        const container = document.getElementById('notifications');

        if (notifications.length === 0) {
            container.innerHTML = '<p class="text-muted">No notifications yet.</p>';
            return;
        }

        let html = '<div class="list-group">';

        notifications.sort((a, b) => new Date(b.created_at) - new Date(a.created_at)).forEach(notification => {
            html += `
                <div class="list-group-item notification-item">
                    <div class="d-flex w-100 justify-content-between">
                        <h6 class="mb-1">${notification.message}</h6>
                        ${getStatusBadge(notification.status)}
                    </div>
                    <div class="d-flex justify-content-between align-items-center mt-2">
                        <small class="text-muted">
                            <strong>ID:</strong> ${notification.id}<br>
                            <strong>Send at:</strong> ${formatDate(notification.send_at)}<br>
                            <strong>Created:</strong> ${formatDate(notification.created_at)}<br>
                            <strong>Attempts:</strong> ${notification.attempts}/${notification.max_retries}
                        </small>
                        <button onclick="deleteNotification('${notification.id}')" class="btn btn-sm btn-danger">Delete</button>
                    </div>
                </div>
            `;
        });

        html += '</div>';
        container.innerHTML = html;
    } catch (error) {
        console.error('Error loading notifications:', error);
        document.getElementById('notifications').innerHTML = '<p class="text-danger">Error loading notifications</p>';
    }
}

// Delete notification
async function deleteNotification(id) {
    if (!confirm('Are you sure you want to delete this notification?')) {
        return;
    }

    try {
        const response = await fetch(`${API_BASE_URL}/notify/${id}`, {
            method: 'DELETE'
        });

        if (response.ok) {
            loadNotifications();
            loadStats();
        } else {
            alert('Failed to delete notification');
        }
    } catch (error) {
        alert(`Error: ${error.message}`);
    }
}

// Load statistics
async function loadStats() {
    try {
        const response = await fetch(`${API_BASE_URL}/metrics`);
        const stats = await response.json();

        const statsContainer = document.getElementById('stats');
        statsContainer.innerHTML = `
            <div class="col-6 mb-2">
                <div class="card text-center">
                    <div class="card-body">
                        <h4 class="card-title">${stats.total || 0}</h4>
                        <p class="card-text text-muted">Total</p>
                    </div>
                </div>
            </div>
            <div class="col-6 mb-2">
                <div class="card text-center">
                    <div class="card-body">
                        <h4 class="card-title">${stats.pending || 0}</h4>
                        <p class="card-text text-muted">Pending</p>
                    </div>
                </div>
            </div>
            <div class="col-4 mb-2">
                <div class="card text-center">
                    <div class="card-body">
                        <h4 class="card-title">${stats.sent || 0}</h4>
                        <p class="card-text text-muted text-success">Sent</p>
                    </div>
                </div>
            </div>
            <div class="col-4 mb-2">
                <div class="card text-center">
                    <div class="card-body">
                        <h4 class="card-title">${stats.failed || 0}</h4>
                        <p class="card-text text-muted text-danger">Failed</p>
                    </div>
                </div>
            </div>
            <div class="col-4 mb-2">
                <div class="card text-center">
                    <div class="card-body">
                        <h4 class="card-title">${stats.retrying || 0}</h4>
                        <p class="card-text text-muted text-warning">Retrying</p>
                    </div>
                </div>
            </div>
        `;
    } catch (error) {
        console.error('Error loading stats:', error);
    }
}

// Initialize
document.addEventListener('DOMContentLoaded', function() {
    setDefaultDateTime();
    document.getElementById('notificationForm').addEventListener('submit', createNotification);
    loadNotifications();
    loadStats();

    // Auto-refresh every 10 seconds
    setInterval(() => {
        loadNotifications();
        loadStats();
    }, 10000);
});