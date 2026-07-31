import collapse from "@alpinejs/collapse";
import focus from "@alpinejs/focus";
import Alpine from "alpinejs";
import "./css/style.css";

import htmx from "htmx.org";

declare global {
  interface Window {
    htmx: typeof htmx;
    Alpine: typeof Alpine;
  }
}

window.htmx = htmx;
window.Alpine = Alpine;

Alpine.plugin(focus);
Alpine.plugin(collapse);
Alpine.start();

document.body.addEventListener("htmx:responseError", function (event: any) {
  const target = event.detail.target; // The element specified by hx-target
  const response = event.detail.xhr.responseText; // The HTML response
  const contentType = event.detail.xhr.getResponseHeader("Content-Type");

  // Check if the response is HTML
  if (target && contentType.includes("text/html")) {
    // Parse the response HTML to extract hidden input values and error message
    const parser = new DOMParser();
    const doc = parser.parseFromString(response, "text/html");
    const modeInput = doc.querySelector('input[name="mode"]');
    const persistenceInput = doc.querySelector('input[name="persistence"]');
    const errorElement = doc.querySelector(
      "div.error-message",
    ) as HTMLDivElement;

    // Default values
    let mode: string = "below"; // Default placement
    let persistence: number = 0; // Default to not disappear

    if (modeInput) {
      mode = modeInput.getAttribute("value") || "below";
    }
    if (persistenceInput) {
      persistence = parseInt(persistenceInput.getAttribute("value") || "0", 10);
    }

    // Remove any existing error message
    const existingError = target.parentNode.querySelector(".error-message");
    if (existingError) {
      existingError.remove();
    }

    // Ensure errorElement exists
    if (!errorElement) {
      return; // Exit if no error-message div is found in the response
    }

    // Handle different modes
    mode = mode.toLowerCase();
    switch (mode) {
      case "replace":
        target.parentNode.replaceChild(errorElement, target);
        break;
      case "above":
        target.parentNode.insertBefore(errorElement, target);
        break;
      case "below":
        target.parentNode.insertBefore(errorElement, target.nextSibling);
        break;
      case "toast_tr":
      case "toast_tm":
      case "toast_tl":
      case "toast_br":
      case "toast_bm":
      case "toast_bl":
        // Add toast-specific styling
        errorElement.style.position = "fixed";
        errorElement.style.zIndex = "1000";
        errorElement.style.padding = "10px";

        // Position based on toast mode
        if (mode === "toast_tr") {
          errorElement.style.top = "10px";
          errorElement.style.right = "10px";
        } else if (mode === "toast_tm") {
          errorElement.style.top = "10px";
          errorElement.style.left = "50%";
          errorElement.style.transform = "translateX(-50%)";
        } else if (mode === "toast_tl") {
          errorElement.style.top = "10px";
          errorElement.style.left = "10px";
        } else if (mode === "toast_br") {
          errorElement.style.bottom = "10px";
          errorElement.style.right = "10px";
        } else if (mode === "toast_bm") {
          errorElement.style.bottom = "10px";
          errorElement.style.left = "50%";
          errorElement.style.transform = "translateX(-50%)";
        } else if (mode === "toast_bl") {
          errorElement.style.bottom = "10px";
          errorElement.style.left = "10px";
        }

        // Append to body for toast modes
        document.body.appendChild(errorElement);
        break;
      default:
        // Fallback to below
        target.parentNode.insertBefore(errorElement, target.nextSibling);
    }

    // Handle persistence (disappear after specified milliseconds)
    if (persistence > 0) {
      setTimeout(() => {
        errorElement.remove();
      }, persistence);
    }
  }
});

// 📡 Register Service Worker for PWA
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker
      .register("/sw.js")
      .then((reg) => {
        console.log("✅ SW registered:", reg.scope); // Should be https://yoursite.com/
      })
      .catch((err) => {
        console.error("❌ SW registration failed:", err);
      });
  });
}
