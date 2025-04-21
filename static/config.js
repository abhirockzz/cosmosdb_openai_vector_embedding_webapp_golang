document.addEventListener("DOMContentLoaded", function () {
  // Load saved configuration if it exists
  loadSavedConfig();

  // Handle form submission
  document
    .getElementById("configForm")
    .addEventListener("submit", function (e) {
      e.preventDefault();
      saveConfig();
    });

  // Initialize all categories as expanded
  document.querySelectorAll(".category-content").forEach((content) => {
    content.classList.remove("collapsed");
  });
  document.querySelectorAll(".category-header").forEach((header) => {
    header.classList.remove("collapsed");
  });
});

function toggleCategory(categoryId) {
  const content = document.getElementById(categoryId);
  const header = content.previousElementSibling;

  content.classList.toggle("collapsed");
  header.classList.toggle("collapsed");
}

function loadSavedConfig() {
  const savedConfig = localStorage.getItem("appConfig");
  if (savedConfig) {
    const config = JSON.parse(savedConfig);
    // Populate form fields with saved values
    Object.keys(config).forEach((key) => {
      const element = document.getElementById(key);
      if (element) {
        element.value = config[key];
      }
    });
  }
}

function saveConfig() {
  const form = document.getElementById("configForm");
  const formData = new FormData(form);
  const config = {};

  formData.forEach((value, key) => {
    // Convert numeric values
    if (key === "chunkSize" || key === "chunkOverlap") {
      config[key] = parseInt(value, 10);
    } else {
      config[key] = value;
    }
  });

  // console.log('Sending configuration:', config);

  // Save to localStorage
  localStorage.setItem("appConfig", JSON.stringify(config));

  // Save to backend
  fetch("/api/config", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(config),
  })
    .then(async (response) => {
      if (!response.ok) {
        // Get the error message from the response if possible
        const errorText = await response.text();
        throw new Error(errorText || `HTTP error! status: ${response.status}`);
      }
      return response.json();
    })
    .then((data) => {
      showStatus("Configuration saved successfully!", "success");
      // Optionally redirect to processing page on success
      window.location.href = "index.html";
    })
    .catch((error) => {
      console.error("Configuration save error:", error);
      showStatus("Error saving configuration: " + error.message, "error");
    });
}

function showStatus(message, type) {
  const statusElement = document.getElementById("status");
  statusElement.textContent = message;
  statusElement.className = "status-message " + type;

  // Clear status after 5 seconds
  setTimeout(() => {
    statusElement.textContent = "";
    statusElement.className = "status-message";
  }, 5000);
}
