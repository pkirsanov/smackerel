package api

import (
	"html/template"
	"log/slog"
	"net/http"

	pwa "github.com/smackerel/smackerel/web/pwa"
)

// pwaFileServer returns an http.Handler that serves embedded PWA static files.
func pwaFileServer() http.Handler {
	return http.FileServerFS(pwa.StaticFiles)
}

// sharePageTemplate is the HTML template served when the OS share target POSTs to /pwa/share.
// It reads the shared data from Go-injected template variables and calls POST /api/capture.
var sharePageTemplate = template.Must(template.New("share").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="theme-color" content="#e94560">
  <title>Saving to Smackerel</title>
  <link rel="stylesheet" href="/pwa/style.css">
</head>
<body>
  <img src="/pwa/icon.svg" alt="Smackerel" class="logo">
  <h1>Smackerel</h1>

  <div class="card" id="saving">
    <div class="status pending">
      <div class="spinner"></div>
      <span>Saving...</span>
    </div>
  </div>

  <div class="card hidden" id="result-success">
    <div class="status success">✅ Saved!</div>
  </div>

  <div class="card hidden" id="result-error">
    <div class="status error" id="error-msg">❌ Save failed</div>
    <button onclick="doCapture()" style="
      margin-top: 0.75rem;
      background: var(--primary);
      color: white;
      border: none;
      border-radius: 8px;
      padding: 0.5rem 1rem;
      cursor: pointer;
    ">Retry</button>
  </div>

  <div class="card hidden" id="queued">
    <div class="status info">📱 Saved offline — will sync when connected</div>
  </div>

  <script src="/pwa/lib/queue.js"></script>
  <script>
    var shareData = {
      title: {{.Title}},
      text:  {{.Text}},
      url:   {{.URL}}
    };

    // Determine the capture URL (shared page might not have explicit server URL)
    var captureURL = '/api/capture';

    function doCapture() {
      document.getElementById('saving').classList.remove('hidden');
      document.getElementById('result-success').classList.add('hidden');
      document.getElementById('result-error').classList.add('hidden');
      document.getElementById('queued').classList.add('hidden');

      var body = {};
      if (shareData.url)   body.url = shareData.url;
      if (shareData.text)  body.text = shareData.text;
      if (shareData.title) body.context = 'Shared: ' + shareData.title;

      // If we got text that looks like a URL, treat it as a URL
      if (!body.url && body.text && /^https?:\/\//.test(body.text.trim())) {
        body.url = body.text.trim();
        delete body.text;
      }

      // Must have at least url or text
      if (!body.url && !body.text) {
        showError('Nothing to capture');
        return;
      }

      // Read auth token from localStorage (set in PWA settings)
      var authToken = localStorage.getItem('smackerel_auth_token') || '';

      fetch(captureURL, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': authToken ? 'Bearer ' + authToken : ''
        },
        body: JSON.stringify(body)
      })
      .then(function(resp) {
        if (resp.ok) {
          showSuccess();
        } else if (resp.status === 409) {
          showSuccess(); // duplicate — still counts as saved
        } else {
          return resp.text().then(function(text) {
            try {
              var data = JSON.parse(text);
              showError(data.error ? data.error.message : 'HTTP ' + resp.status);
            } catch (e) {
              showError('HTTP ' + resp.status);
            }
          });
        }
      })
      .catch(function(err) {
        // Offline — queue for later using shared IndexedDB CaptureQueue
        queueOffline(shareData);
      });
    }

    function showSuccess() {
      document.getElementById('saving').classList.add('hidden');
      document.getElementById('result-success').classList.remove('hidden');
      // Auto-close after 1.5s to return to source app
      setTimeout(function() { window.close(); }, 1500);
    }

    function showError(msg) {
      document.getElementById('saving').classList.add('hidden');
      document.getElementById('error-msg').textContent = '❌ ' + msg;
      document.getElementById('result-error').classList.remove('hidden');
    }

    function queueOffline(data) {
      // Use the shared IndexedDB CaptureQueue so the service worker can sync later
      CaptureQueue.enqueue({
        url: data.url || '',
        title: data.title || '',
        text: data.text || ''
      }).then(function() {
        document.getElementById('saving').classList.add('hidden');
        document.getElementById('queued').classList.remove('hidden');
        setTimeout(function() { window.close(); }, 2000);
      }).catch(function() {
        // IndexedDB unavailable — show error instead of silently losing the item
        showError('Offline and unable to queue — please try again');
      });
    }

    // Start capture immediately
    doCapture();
  </script>
</body>
</html>`))

// PWAShareHandler handles POST /pwa/share from the OS Web Share Target API.
// It parses form-encoded data (title, text, url) and serves an HTML page
// that captures the content via the existing /api/capture endpoint.
func (d *Dependencies) PWAShareHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		slog.Warn("pwa share: bad form data", "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	data := struct {
		Title string
		Text  string
		URL   string
	}{
		Title: r.FormValue("title"),
		Text:  r.FormValue("text"),
		URL:   r.FormValue("url"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := sharePageTemplate.Execute(w, data); err != nil {
		slog.Error("pwa share: template render failed", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}
