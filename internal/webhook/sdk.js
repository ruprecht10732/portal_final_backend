/**
 * RAC Form Capture SDK
 * 
 * Automatically captures form submissions and sends them to your lead management system.
 * 
 * Usage:
 *   <script src="https://your-api.com/api/v1/webhook/sdk.js" data-api-key="whk_..." async></script>
 * 
 * Options (data attributes on the script tag):
 *   data-api-key      (required) Your webhook API key
 *   data-endpoint     (optional) Custom API endpoint URL (defaults to script origin)
 *   data-selector     (optional) CSS selector for forms to capture (defaults to all forms)
 *   data-success-url  (optional) Redirect URL after successful submission
 *   data-tracking-ttl (optional) Tracking TTL in days (default: 90)
 * 
 * Manual capture (for JS-rendered forms):
 *   window.RACFormCapture.submit({ name: "John", email: "john@example.com" })
 */
(function () {
  'use strict';

  // Find our script tag to read config
  var scripts = document.querySelectorAll('script[data-api-key]');
  var scriptTag = scripts[scripts.length - 1];
  if (!scriptTag) {
    console.warn('[RAC] No script tag with data-api-key found');
    return;
  }

  var config = {
    apiKey: scriptTag.getAttribute('data-api-key'),
    endpoint: scriptTag.getAttribute('data-endpoint') || getScriptOrigin(scriptTag),
    selector: scriptTag.getAttribute('data-selector') || null,
    successUrl: scriptTag.getAttribute('data-success-url') || null,
    trackingTTL: parseInt(scriptTag.getAttribute('data-tracking-ttl') || '90', 10)
  };

  if (!config.apiKey) {
    console.warn('[RAC] data-api-key is required');
    return;
  }

  function getScriptOrigin(tag) {
    try {
      var url = new URL(tag.src);
      return url.origin + '/api/v1/webhook/forms';
    } catch (e) {
      return '/api/v1/webhook/forms';
    }
  }

  var STORAGE_KEY = 'rac_tracking_data';

  function captureTrackingParams() {
    try {
      var params = new URLSearchParams(window.location.search);
      var trackingData = loadTrackingData() || {};
      var hasUpdates = false;

      var gclid = params.get('gclid');
      if (gclid) {
        trackingData.gclid = gclid;
        hasUpdates = true;
      }

      var utmParams = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_content', 'utm_term'];
      utmParams.forEach(function (param) {
        var value = params.get(param);
        if (value) {
          trackingData[param] = value;
          hasUpdates = true;
        }
      });

      if (gclid || !trackingData.ad_landing_page) {
        trackingData.ad_landing_page = window.location.href;
        hasUpdates = true;
      }

      if (!trackingData.referrer_url && document.referrer) {
        trackingData.referrer_url = document.referrer;
        hasUpdates = true;
      }

      if (hasUpdates) {
        var expiryDate = new Date();
        expiryDate.setDate(expiryDate.getDate() + config.trackingTTL);
        trackingData.expiry = expiryDate.toISOString();

        localStorage.setItem(STORAGE_KEY, JSON.stringify(trackingData));
      }
    } catch (e) {
      console.warn('[RAC] Failed to capture tracking params:', e && e.message ? e.message : e);
    }
  }

  function loadTrackingData() {
    try {
      var stored = localStorage.getItem(STORAGE_KEY);
      if (!stored) return null;

      var data = JSON.parse(stored);
      if (data && data.expiry) {
        var expiry = new Date(data.expiry);
        if (expiry < new Date()) {
          localStorage.removeItem(STORAGE_KEY);
          return null;
        }
      }

      if (data && data.expiry) {
        delete data.expiry;
      }
      return data;
    } catch (e) {
      return null;
    }
  }

  function appendTrackingData(data) {
    var trackingData = loadTrackingData();
    if (!trackingData) return data;

    if (data instanceof FormData) {
      Object.keys(trackingData).forEach(function (key) {
        if (!data.has(key)) {
          data.append(key, trackingData[key]);
        }
      });
    } else {
      Object.keys(trackingData).forEach(function (key) {
        if (!(key in data)) {
          data[key] = trackingData[key];
        }
      });
    }

    return data;
  }

  /**
   * Submit form data to the webhook endpoint.
   * @param {Object|FormData} data - Key-value pairs or FormData object
   * @param {Object} [options] - Optional overrides
   * @returns {Promise<Object>} Response from the API
   */
  function submit(data, options) {
    options = options || {};
    var url = options.endpoint || config.endpoint;
    var apiKey = options.apiKey || config.apiKey;

    data = appendTrackingData(data);

    var body;
    var headers = {
      'X-Webhook-API-Key': apiKey
    };

    if (data instanceof FormData) {
      body = data;
      // Don't set Content-Type — browser will set multipart/form-data with boundary
    } else {
      body = JSON.stringify(data);
      headers['Content-Type'] = 'application/json';
    }

    return fetch(url, {
      method: 'POST',
      headers: headers,
      body: body,
      mode: 'cors'
    }).then(function (response) {
      if (!response.ok) {
        return response.json().then(function (err) {
          throw new Error(err.error || 'Form submission failed');
        });
      }
      return response.json();
    });
  }

  /**
   * Auto-capture form submissions
   */
  function attachFormListeners() {
    var selector = config.selector || 'form';
    var forms = document.querySelectorAll(selector);

    forms.forEach(function (form) {
      // Skip forms that are already captured or explicitly excluded
      if (form.hasAttribute('data-rac-ignore') || form.hasAttribute('data-rac-attached')) {
        return;
      }
      form.setAttribute('data-rac-attached', 'true');

      form.addEventListener('submit', function (e) {
        e.preventDefault();

        var formData = new FormData(form);

        // Show loading state
        var submitBtn = form.querySelector('[type="submit"]');
        var originalText = '';
        if (submitBtn) {
          originalText = submitBtn.textContent;
          submitBtn.textContent = 'Verzenden...';
          submitBtn.disabled = true;
        }

        submit(formData)
          .then(function (response) {
            // Success
            if (config.successUrl) {
              window.location.href = config.successUrl;
            } else {
              // Dispatch custom event for app-level handling
              form.dispatchEvent(new CustomEvent('rac:success', {
                detail: response,
                bubbles: true
              }));

              // Default: show simple success message
              if (!form.hasAttribute('data-rac-no-message')) {
                showMessage(form, 'Bedankt! Uw aanvraag is ontvangen.', 'success');
              }
              form.reset();
            }
          })
          .catch(function (error) {
            form.dispatchEvent(new CustomEvent('rac:error', {
              detail: { error: error.message },
              bubbles: true
            }));

            if (!form.hasAttribute('data-rac-no-message')) {
              showMessage(form, 'Er is iets misgegaan. Probeer het opnieuw.', 'error');
            }
          })
          .finally(function () {
            if (submitBtn) {
              submitBtn.textContent = originalText;
              submitBtn.disabled = false;
            }
          });
      });
    });
  }

  function showMessage(form, text, type) {
    // Remove previous message
    var existing = form.querySelector('.rac-message');
    if (existing) existing.remove();

    var msg = document.createElement('div');
    msg.className = 'rac-message rac-message--' + type;
    msg.textContent = text;
    msg.style.cssText = 'padding:12px;margin:8px 0;border-radius:6px;font-size:14px;' +
      (type === 'success'
        ? 'background:#d4edda;color:#155724;border:1px solid #c3e6cb;'
        : 'background:#f8d7da;color:#721c24;border:1px solid #f5c6cb;');
    form.appendChild(msg);

    setTimeout(function () { msg.remove(); }, 5000);
  }

  // Expose public API
  window.RACFormCapture = {
    submit: submit,
    refresh: attachFormListeners,
    getTrackingData: loadTrackingData,
    clearTrackingData: function () {
      localStorage.removeItem(STORAGE_KEY);
    }
  };

  captureTrackingParams();

  // Auto-attach when DOM is ready
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', attachFormListeners);
  } else {
    attachFormListeners();
  }

  // Re-attach on dynamic content (MutationObserver for SPAs)
  if (typeof MutationObserver !== 'undefined') {
    var observer = new MutationObserver(function (mutations) {
      var hasNewForms = mutations.some(function (m) {
        return Array.from(m.addedNodes).some(function (n) {
          return n.nodeType === 1 && (n.tagName === 'FORM' || n.querySelector && n.querySelector('form'));
        });
      });
      if (hasNewForms) {
        attachFormListeners();
      }
    });
    observer.observe(document.body || document.documentElement, {
      childList: true,
      subtree: true
    });
  }
})();
