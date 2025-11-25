// WebRTC Audio Call Manager for The Skyscape
(function() {
    'use strict';

    window.Skyscape = window.Skyscape || {};

    const CallManager = {
        peerConnection: null,
        localStream: null,
        eventSource: null,
        currentCall: null,
        iceServers: [],
        callTimerInterval: null,
        callStartTime: null,

        // Initialize SSE connection for call events
        init: function() {
            this.initSSE();
            this.getTURNCredentials();
            this.setupMessageListener();
        },

        reconnectAttempts: 0,
        maxReconnectDelay: 30000,
        reconnectTimer: null,

        initSSE: function() {
            // Clear any pending reconnect
            if (this.reconnectTimer) {
                clearTimeout(this.reconnectTimer);
                this.reconnectTimer = null;
            }

            if (this.eventSource) {
                this.eventSource.close();
                this.eventSource = null;
            }

            this.eventSource = new EventSource('/calls/events');

            this.eventSource.onopen = () => {
                console.log('[Call] SSE connected');
                this.reconnectAttempts = 0; // Reset on successful connection
            };

            this.eventSource.onerror = (err) => {
                console.error('[Call] SSE error:', err);

                // Close the failed connection
                if (this.eventSource) {
                    this.eventSource.close();
                    this.eventSource = null;
                }

                // Exponential backoff: 5s, 10s, 20s, 30s max
                this.reconnectAttempts++;
                const delay = Math.min(5000 * Math.pow(2, this.reconnectAttempts - 1), this.maxReconnectDelay);
                console.log(`[Call] Reconnecting in ${delay/1000}s (attempt ${this.reconnectAttempts})`);

                this.reconnectTimer = setTimeout(() => this.initSSE(), delay);
            };

            // Call event listeners
            this.eventSource.addEventListener('call_incoming', (e) => this.handleIncomingCall(e));
            this.eventSource.addEventListener('call_accepted', (e) => this.handleCallAccepted(e));
            this.eventSource.addEventListener('call_ended', (e) => this.handleCallEnded(e));
            this.eventSource.addEventListener('sdp_offer', (e) => this.handleSDPOffer(e));
            this.eventSource.addEventListener('sdp_answer', (e) => this.handleSDPAnswer(e));
            this.eventSource.addEventListener('ice_candidate', (e) => this.handleICECandidate(e));
        },

        async getTURNCredentials() {
            try {
                const resp = await fetch('/calls/turn-credentials', { credentials: 'same-origin' });
                if (resp.ok) {
                    const data = await resp.json();
                    this.iceServers = data.iceServers || [];
                    console.log('[Call] TURN credentials loaded');
                }
            } catch (err) {
                console.error('[Call] Failed to get TURN credentials:', err);
            }
        },

        setupMessageListener() {
            // Listen for messages from service worker (call actions)
            navigator.serviceWorker?.addEventListener('message', (event) => {
                if (event.data.type === 'call_action') {
                    const { action, callId } = event.data;
                    if (action === 'accept' && this.currentCall?.id === callId) {
                        this.accept();
                    } else if (action === 'reject' && this.currentCall?.id === callId) {
                        this.reject();
                    }
                }
            });
        },

        // Handle incoming call
        handleIncomingCall(event) {
            const data = JSON.parse(event.data);
            this.currentCall = { id: data.callId, role: 'callee' };

            const caller = data.payload;
            this.showIncomingCallModal(caller);

            // Show notification if page is hidden
            if (document.hidden && window.Skyscape.notify) {
                window.Skyscape.notify('Incoming Call', {
                    body: `${caller.callerName} is calling you`,
                    tag: 'incoming-call-' + data.callId,
                    data: { url: window.location.href, callId: data.callId },
                    requireInteraction: true
                });
            }
        },

        // Handle call accepted by callee
        async handleCallAccepted(event) {
            console.log('[Call] Call accepted, creating offer');
            this.updateCallUI('connecting');
            await this.createOffer();
        },

        // Handle call ended
        handleCallEnded(event) {
            const data = JSON.parse(event.data);
            const { reason, duration } = data.payload;
            this.cleanup();
            this.showCallEndedToast(reason, duration);
        },

        // Handle SDP offer from caller
        async handleSDPOffer(event) {
            const data = JSON.parse(event.data);
            console.log('[Call] Received SDP offer');

            await this.createPeerConnection();

            const offer = new RTCSessionDescription({
                type: data.payload.type,
                sdp: data.payload.sdp
            });

            await this.peerConnection.setRemoteDescription(offer);
            const answer = await this.peerConnection.createAnswer();
            await this.peerConnection.setLocalDescription(answer);

            // Send answer back
            await fetch(`/calls/${this.currentCall.id}/sdp`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ type: 'answer', sdp: answer.sdp })
            });
        },

        // Handle SDP answer from callee
        async handleSDPAnswer(event) {
            const data = JSON.parse(event.data);
            console.log('[Call] Received SDP answer');

            const answer = new RTCSessionDescription({
                type: data.payload.type,
                sdp: data.payload.sdp
            });
            await this.peerConnection.setRemoteDescription(answer);
        },

        // Handle ICE candidate
        async handleICECandidate(event) {
            const data = JSON.parse(event.data);
            if (this.peerConnection && data.payload.candidate) {
                try {
                    await this.peerConnection.addIceCandidate(new RTCIceCandidate({
                        candidate: data.payload.candidate,
                        sdpMid: data.payload.sdpMid,
                        sdpMLineIndex: data.payload.sdpMLineIndex
                    }));
                } catch (err) {
                    console.error('[Call] Failed to add ICE candidate:', err);
                }
            }
        },

        // Create peer connection
        async createPeerConnection() {
            const config = {
                iceServers: this.iceServers.length > 0 ? this.iceServers : [
                    { urls: 'stun:stun.l.google.com:19302' }
                ]
            };

            this.peerConnection = new RTCPeerConnection(config);

            // Get local audio stream
            this.localStream = await navigator.mediaDevices.getUserMedia({
                audio: true,
                video: false
            });
            this.localStream.getTracks().forEach(track => {
                this.peerConnection.addTrack(track, this.localStream);
            });

            // Handle remote stream
            this.peerConnection.ontrack = (event) => {
                console.log('[Call] Received remote track');
                const remoteAudio = document.getElementById('remote-audio');
                if (remoteAudio) {
                    remoteAudio.srcObject = event.streams[0];
                }
                this.updateCallUI('connected');
            };

            // Handle ICE candidates
            this.peerConnection.onicecandidate = async (event) => {
                if (event.candidate) {
                    await fetch(`/calls/${this.currentCall.id}/ice`, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        credentials: 'same-origin',
                        body: JSON.stringify({
                            candidate: event.candidate.candidate,
                            sdpMid: event.candidate.sdpMid,
                            sdpMLineIndex: event.candidate.sdpMLineIndex
                        })
                    });
                }
            };

            // Handle connection state changes
            this.peerConnection.onconnectionstatechange = () => {
                console.log('[Call] Connection state:', this.peerConnection.connectionState);
                if (this.peerConnection.connectionState === 'failed') {
                    this.end();
                }
            };

            return this.peerConnection;
        },

        // Create and send offer
        async createOffer() {
            await this.createPeerConnection();
            const offer = await this.peerConnection.createOffer();
            await this.peerConnection.setLocalDescription(offer);

            await fetch(`/calls/${this.currentCall.id}/sdp`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ type: 'offer', sdp: offer.sdp })
            });
        },

        // Initiate a call to user
        async call(userId) {
            try {
                const resp = await fetch(`/calls/${userId}/initiate`, {
                    method: 'POST',
                    credentials: 'same-origin'
                });

                if (!resp.ok) {
                    const error = await resp.json();
                    throw new Error(error.error || 'Failed to initiate call');
                }

                const data = await resp.json();
                this.currentCall = { id: data.callId, role: 'caller' };
                this.showOutgoingCallModal();
            } catch (err) {
                this.showErrorToast(err.message);
            }
        },

        // Accept incoming call
        async accept() {
            if (!this.currentCall) return;

            try {
                const resp = await fetch(`/calls/${this.currentCall.id}/accept`, {
                    method: 'POST',
                    credentials: 'same-origin'
                });

                if (resp.ok) {
                    this.hideIncomingCallModal();
                    this.showActiveCallModal();
                }
            } catch (err) {
                console.error('[Call] Failed to accept:', err);
            }
        },

        // Reject incoming call or cancel outgoing
        async reject() {
            if (!this.currentCall) return;

            try {
                await fetch(`/calls/${this.currentCall.id}/reject`, {
                    method: 'POST',
                    credentials: 'same-origin'
                });
            } catch (err) {
                console.error('[Call] Failed to reject:', err);
            }
            this.cleanup();
        },

        // End active call
        async end() {
            if (!this.currentCall) return;

            try {
                await fetch(`/calls/${this.currentCall.id}/end`, {
                    method: 'POST',
                    credentials: 'same-origin'
                });
            } catch (err) {
                console.error('[Call] Failed to end:', err);
            }
            this.cleanup();
        },

        // Toggle mute
        toggleMute() {
            if (this.localStream) {
                const audioTrack = this.localStream.getAudioTracks()[0];
                if (audioTrack) {
                    audioTrack.enabled = !audioTrack.enabled;
                    return !audioTrack.enabled; // Return true if muted
                }
            }
            return false;
        },

        // Cleanup call resources
        cleanup() {
            if (this.localStream) {
                this.localStream.getTracks().forEach(track => track.stop());
                this.localStream = null;
            }
            if (this.peerConnection) {
                this.peerConnection.close();
                this.peerConnection = null;
            }
            this.currentCall = null;
            this.stopCallTimer();
            this.hideAllModals();
        },

        // UI Methods
        showIncomingCallModal(caller) {
            const modal = document.getElementById('call-incoming-modal');
            if (modal) {
                const nameEl = modal.querySelector('.caller-name');
                const avatarEl = modal.querySelector('.caller-avatar');
                if (nameEl) nameEl.textContent = caller.callerName;
                if (avatarEl) avatarEl.src = caller.callerAvatar;
                modal.showModal();
            }
        },

        hideIncomingCallModal() {
            const modal = document.getElementById('call-incoming-modal');
            if (modal) modal.close();
        },

        showOutgoingCallModal() {
            const modal = document.getElementById('call-outgoing-modal');
            if (modal) modal.showModal();
        },

        showActiveCallModal() {
            const modal = document.getElementById('call-active-modal');
            if (modal) {
                modal.showModal();
                this.startCallTimer();
            }
        },

        hideAllModals() {
            ['call-incoming-modal', 'call-outgoing-modal', 'call-active-modal'].forEach(id => {
                const modal = document.getElementById(id);
                if (modal) modal.close();
            });
        },

        updateCallUI(state) {
            const statusEl = document.querySelector('.call-status');
            if (statusEl) {
                if (state === 'connecting') statusEl.textContent = 'Connecting...';
                if (state === 'connected') {
                    statusEl.textContent = 'Connected';
                    this.startCallTimer();
                }
            }
        },

        startCallTimer() {
            this.callStartTime = Date.now();
            const timerEl = document.querySelector('.call-timer');
            this.callTimerInterval = setInterval(() => {
                const elapsed = Math.floor((Date.now() - this.callStartTime) / 1000);
                if (timerEl) timerEl.textContent = this.formatDuration(elapsed);
            }, 1000);
        },

        stopCallTimer() {
            if (this.callTimerInterval) {
                clearInterval(this.callTimerInterval);
                this.callTimerInterval = null;
            }
        },

        formatDuration(seconds) {
            const mins = Math.floor(seconds / 60);
            const secs = seconds % 60;
            return `${mins}:${secs.toString().padStart(2, '0')}`;
        },

        showCallEndedToast(reason, duration) {
            let message = 'Call ended';
            if (reason === 'completed' && duration) {
                message = `Call ended (${this.formatDuration(duration)})`;
            } else if (reason === 'rejected') {
                message = 'Call declined';
            } else if (reason === 'cancelled') {
                message = 'Call cancelled';
            } else if (reason === 'missed') {
                message = 'Missed call';
            }
            this.showToast(message, 'info');
        },

        showErrorToast(message) {
            this.showToast(message, 'error');
        },

        showToast(message, type = 'info') {
            const toast = document.createElement('div');
            toast.className = 'toast toast-end z-50';
            toast.innerHTML = `<div class="alert alert-${type}"><span>${message}</span></div>`;
            document.body.appendChild(toast);
            setTimeout(() => toast.remove(), 4000);
        }
    };

    // Initialize on DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => CallManager.init());
    } else {
        CallManager.init();
    }

    // Expose to global scope
    window.Skyscape.CallManager = CallManager;
})();
