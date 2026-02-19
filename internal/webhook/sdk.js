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

  var runtimeConfig = {
    gtmContainerId: null
  };

  function getConfigEndpoint() {
    try {
      var endpointUrl = new URL(config.endpoint, window.location.href);
      return endpointUrl.origin + '/api/v1/webhook/config';
    } catch (e) {
      return '/api/v1/webhook/config';
    }
  }

  function fetchRuntimeConfig() {
    var url = getConfigEndpoint();

    return fetch(url, {
      method: 'GET',
      headers: {
        'X-Webhook-API-Key': config.apiKey
      },
      mode: 'cors'
    }).then(function (response) {
      if (!response.ok) {
        throw new Error('Failed to load SDK config');
      }
      return response.json();
    }).then(function (cfg) {
      runtimeConfig.gtmContainerId = cfg && cfg.gtmContainerId ? String(cfg.gtmContainerId) : null;
      if (runtimeConfig.gtmContainerId) {
        loadGTM(runtimeConfig.gtmContainerId);
      }
      return runtimeConfig;
    }).catch(function (e) {
      console.warn('[RAC] Failed to load runtime config:', e && e.message ? e.message : e);
      return runtimeConfig;
    });
  }

  function loadGTM(containerId) {
    try {
      if (!containerId) return;

      // If GTM is already present on the page, don't inject a second time.
      if (window.google_tag_manager && window.google_tag_manager[containerId]) {
        return;
      }
      var existingBySrc = document.querySelector('script[src*="googletagmanager.com/gtm.js?id=' + containerId + '"]');
      if (existingBySrc) return;

      // Avoid double-injecting
      var existing = document.getElementById('rac-gtm-loader');
      if (existing) return;

      window.dataLayer = window.dataLayer || [];
      window.dataLayer.push({ 'gtm.start': new Date().getTime(), event: 'gtm.js' });

      var firstScript = document.getElementsByTagName('script')[0];
      var gtmScript = document.createElement('script');
      gtmScript.async = true;
      gtmScript.id = 'rac-gtm-loader';
      gtmScript.src = 'https://www.googletagmanager.com/gtm.js?id=' + encodeURIComponent(containerId);
      if (firstScript && firstScript.parentNode) {
        firstScript.parentNode.insertBefore(gtmScript, firstScript);
      } else {
        document.head.appendChild(gtmScript);
      }
    } catch (e) {
      console.warn('[RAC] Failed to load GTM:', e && e.message ? e.message : e);
    }
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

  // ---- Enhanced Conversions (dataLayer push) ----

  var emailRegex = /^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$/;

  var firstNamePatterns = ['first_name', 'firstname', 'first name', 'voornaam', 'given_name', 'givenname', 'fname'];
  var lastNamePatterns = ['last_name', 'lastname', 'last name', 'achternaam', 'family_name', 'familyname', 'surname', 'lname'];
  var fullNamePatterns = ['name', 'naam', 'full_name', 'fullname', 'your_name', 'your name'];
  var emailPatterns = ['email', 'e-mail', 'e_mail', 'emailaddress', 'email_address', 'mail'];
  var phonePatterns = ['phone', 'telefoon', 'tel', 'telephone', 'phonenumber', 'phone_number', 'telefoonnummer', 'mobile', 'mobiel', 'gsm'];

  function normalizeKey(label) {
    return String(label || '')
      .toLowerCase()
      .replace(/[\-\_\s]/g, '');
  }

  function matchesAny(label, patterns) {
    var normalized = normalizeKey(label);
    for (var i = 0; i < patterns.length; i++) {
      if (normalized === normalizeKey(patterns[i])) {
        return true;
      }
    }
    return false;
  }

  function normalizePhone(value) {
    var input = String(value || '').trim();
    if (!input) return '';

    var cleaned = input.replace(/[^0-9+]/g, '');

    // Dutch normalization similar to backend extractor
    if (cleaned.indexOf('06') === 0 && cleaned.length === 10) {
      return '+31' + cleaned.substring(1);
    }
    if (cleaned.indexOf('0031') === 0) {
      return '+' + cleaned.substring(2);
    }
    if (cleaned.indexOf('0') === 0 && cleaned.length === 10) {
      return '+31' + cleaned.substring(1);
    }

    return cleaned;
  }

  function splitFullName(fullName) {
    var v = String(fullName || '').trim();
    if (!v) return { firstName: '', lastName: '' };
    var idx = v.indexOf(' ');
    if (idx === -1) {
      return { firstName: v, lastName: '' };
    }
    return { firstName: v.substring(0, idx), lastName: v.substring(idx + 1).trim() };
  }

  function extractEnhancedConversions(data) {
    var fields = {
      firstName: '',
      lastName: '',
      email: '',
      phone: ''
    };

    function applyField(key, value) {
      var k = String(key || '');
      var v = String(value || '').trim();
      if (!v) return;

      if (matchesAny(k, firstNamePatterns)) {
        fields.firstName = v;
        return;
      }
      if (matchesAny(k, lastNamePatterns)) {
        fields.lastName = v;
        return;
      }
      if (matchesAny(k, fullNamePatterns)) {
        var parts = splitFullName(v);
        fields.firstName = parts.firstName;
        if (parts.lastName) fields.lastName = parts.lastName;
        return;
      }
      if (matchesAny(k, emailPatterns)) {
        var email = v.toLowerCase();
        if (emailRegex.test(email)) {
          fields.email = email;
        }
        return;
      }
      if (matchesAny(k, phonePatterns)) {
        fields.phone = normalizePhone(v);
      }
    }

    if (data && typeof FormData !== 'undefined' && data instanceof FormData) {
      data.forEach(function (value, key) {
        applyField(key, value);
      });
    } else if (data && typeof data === 'object') {
      Object.keys(data).forEach(function (key) {
        applyField(key, data[key]);
      });
    }

    // If we got a full name but no separate last name and first looks like "first last"
    if (fields.firstName && !fields.lastName && fields.firstName.indexOf(' ') !== -1) {
      var split = splitFullName(fields.firstName);
      fields.firstName = split.firstName;
      fields.lastName = split.lastName;
    }

    var ec = {};
    if (fields.email) ec.email = fields.email;
    if (fields.phone) ec.phone_number = fields.phone;
    if (fields.firstName) ec.first_name = fields.firstName;
    if (fields.lastName) ec.last_name = fields.lastName;
    return ec;
  }

  function pushGenerateLeadEvent(data) {
    try {
      window.dataLayer = window.dataLayer || [];

      var ec = extractEnhancedConversions(data);
      var payload = { event: 'generate_lead' };
      if (ec && Object.keys(ec).length > 0) {
        payload.enhanced_conversions = ec;
      }

      window.dataLayer.push(payload);
    } catch (e) {
      // Never block submit flow
    }
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
      'X-Webhook-API-Key': apiKey,
      'X-Idempotency-Key': options.idempotencyKey || (Date.now().toString() + '-' + Math.random().toString(36).substr(2, 9))
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
    }).then(function (response) {
      // Push conversion event only after successful submit
      pushGenerateLeadEvent(data);
      return response;
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

      // Skip forms that already submit to our endpoint natively
      var action = form.getAttribute('action');
      if (action && action.indexOf(config.endpoint) !== -1) {
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

  // Load runtime config (GTM container) ASAP, but don't block form capture.
  fetchRuntimeConfig();

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
