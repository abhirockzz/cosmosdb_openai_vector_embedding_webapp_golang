// Function to toggle category visibility
function toggleCategory(categoryId) {
  const content = document.getElementById(categoryId);
  const header = content.previousElementSibling;

  content.classList.toggle("collapsed");
  header.classList.toggle("collapsed");
}

// Add smooth scroll to sections
function smoothScrollTo(elementId) {
  document.getElementById(elementId).scrollIntoView({ behavior: "smooth" });
}

// Global configuration object
let currentConfig = {
  cosmosDBEndpoint: "",
  databaseName: "",
  containerName: "",
  azureOpenAIEndpoint: "",
  embeddingModel: "text-embedding-ada-002",
  textAttribute: "text",
  embeddingAttribute: "embedding",
  metadataAttribute: "metadata",
  chunkSize: 1000,
  chunkOverlap: 200,
};

// Store configuration to localStorage
function saveConfig() {
  localStorage.setItem("embeddingConfig", JSON.stringify(currentConfig));
}

// Load configuration from localStorage
function loadConfig() {
  const savedConfig = localStorage.getItem("embeddingConfig");
  if (savedConfig) {
    currentConfig = JSON.parse(savedConfig);

    // Update form fields with saved values
    document.getElementById("cosmosDBEndpoint").value =
      currentConfig.cosmosDBEndpoint;
    document.getElementById("databaseName").value = currentConfig.databaseName;
    document.getElementById("containerName").value =
      currentConfig.containerName;
    document.getElementById("azureOpenAIEndpoint").value =
      currentConfig.azureOpenAIEndpoint;
    document.getElementById("embeddingModel").value =
      currentConfig.embeddingModel;
    document.getElementById("textAttribute").value =
      currentConfig.textAttribute;
    document.getElementById("embeddingAttribute").value =
      currentConfig.embeddingAttribute;
    document.getElementById("metadataAttribute").value =
      currentConfig.metadataAttribute;
    document.getElementById("chunkSize").value = currentConfig.chunkSize;
    document.getElementById("chunkOverlap").value = currentConfig.chunkOverlap;
  }
}

// Update current configuration with form values
function updateConfig() {
  currentConfig.cosmosDBEndpoint = document.getElementById(
    "cosmosDBEndpoint"
  ).value;
  currentConfig.databaseName = document.getElementById("databaseName").value;
  currentConfig.containerName = document.getElementById("containerName").value;
  currentConfig.azureOpenAIEndpoint = document.getElementById(
    "azureOpenAIEndpoint"
  ).value;
  currentConfig.embeddingModel = document.getElementById(
    "embeddingModel"
  ).value;
  currentConfig.textAttribute = document.getElementById("textAttribute").value;
  currentConfig.embeddingAttribute = document.getElementById(
    "embeddingAttribute"
  ).value;
  currentConfig.metadataAttribute = document.getElementById(
    "metadataAttribute"
  ).value;
  currentConfig.chunkSize =
    parseInt(document.getElementById("chunkSize").value) || 1000;
  currentConfig.chunkOverlap =
    parseInt(document.getElementById("chunkOverlap").value) || 200;

  // Save updated config
  saveConfig();
}

document.addEventListener("DOMContentLoaded", () => {
  const startButton = document.getElementById("startButton");
  const progressFill = document.getElementById("progressFill");
  const progressStatus = document.getElementById("progressStatus");
  const urlInput = document.getElementById("urlInput");
  const fileInput = document.getElementById("fileInput");
  const urlInputContainer = document.getElementById("urlInputContainer");
  const fileInputContainer = document.getElementById("fileInputContainer");
  const urlOption = document.getElementById("urlOption");
  const fileOption = document.getElementById("fileOption");
  const configStatus = document.getElementById("configStatus");
  const mainContent = document.getElementById("mainContent");

  let progressInterval;
  let processingStarted = false;

  // Check if configuration exists
  const config = localStorage.getItem("appConfig");
  if (!config) {
    configStatus.style.display = "block";
    mainContent.style.display = "none";
    return;
  }

  // Show main content and hide config status
  configStatus.style.display = "none";
  mainContent.style.display = "block";

  // Load saved configuration on page load
  loadConfig();

  // Handle input type selection
  function handleInputTypeChange() {
    if (urlOption.checked) {
      urlInputContainer.style.display = "block";
      fileInputContainer.style.display = "none";
      fileInput.value = ""; // Clear file input
    } else if (fileOption.checked) {
      urlInputContainer.style.display = "none";
      fileInputContainer.style.display = "block";
      urlInput.value = ""; // Clear URL input
    } else {
      urlInputContainer.style.display = "none";
      fileInputContainer.style.display = "none";
    }
  }

  urlOption.addEventListener("change", handleInputTypeChange);
  fileOption.addEventListener("change", handleInputTypeChange);

  startButton.addEventListener("click", async () => {
    // Disable the start button
    startButton.disabled = true;
    progressStatus.textContent = "Starting...";
    progressFill.style.width = "0%";
    processingStarted = false;

    // Check if an input type is selected and has a value
    if (!urlOption.checked && !fileOption.checked) {
      progressStatus.textContent = "Please select an input type (URL or Files)";
      startButton.disabled = false;
      return;
    }

    if (urlOption.checked && !urlInput.value) {
      progressStatus.textContent = "Please enter a URL";
      startButton.disabled = false;
      return;
    }

    if (fileOption.checked && fileInput.files.length === 0) {
      progressStatus.textContent = "Please select at least one file";
      startButton.disabled = false;
      return;
    }

    const formData = new FormData();
    
    // Add URL if selected
    if (urlOption.checked) {
      formData.append("url", urlInput.value);
    }

    // Add files if selected
    if (fileOption.checked) {
      for (const file of fileInput.files) {
        formData.append("files", file);
      }
    }

    try {
      // Start progress polling
      startProgressPolling();

      // Send the request to process the files
      const response = await fetch("/api/process", {
        method: "POST",
        body: formData,
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`HTTP error! status: ${response.status}, message: ${errorText}`);
      }

      processingStarted = true;

      // Wait for processing to complete
      const result = await response.text();
      
      if (result !== "Files processed successfully" && result !== "URL processed successfully") {
        throw new Error(`Unexpected response: ${result}`);
      }

      // Ensure progress bar is filled completely when processing is done
      progressFill.style.width = "100%";
      progressStatus.textContent = "Processing completed successfully!";
    } catch (error) {
      console.error("Error:", error);
      progressStatus.textContent = `Error: ${error.message}`;
      progressStatus.className = "progress-status error";
    } finally {
      startButton.disabled = false;
      stopProgressPolling();
    }
  });

  function startProgressPolling() {
    let lastProgressPercentage = 0;

    progressInterval = setInterval(async () => {
      try {
        const response = await fetch("/api/progress");
        const progress = await response.json();

        if (progress.error) {
          progressStatus.textContent = `Error: ${progress.error}`;
          progressStatus.className = "progress-status error";
          startButton.disabled = false;
          stopProgressPolling();
          return;
        }

        // Calculate percentage but never decrease it
        let percentage = 0;

        // For single file/URL processing or when total is 0
        if (progress.total <= 1 || progress.total === 0) {
          if (!processingStarted) {
            // For single file, gradually increase progress for visual feedback
            lastProgressPercentage += 5;
            percentage = Math.min(90, lastProgressPercentage); // Cap at 90% until complete
          } else {
            percentage = 100; // If processing started, show complete
          }
        } else {
          // For multiple files, calculate percentage based on progress
          const calculatedPercentage = progress.total > 0 ? (progress.processed / progress.total) * 100 : 0;
          
          // Never decrease the progress percentage
          percentage = Math.max(calculatedPercentage, lastProgressPercentage);
          lastProgressPercentage = percentage;
        }

        progressFill.style.width = `${percentage}%`;
        progressStatus.textContent = progress.status;

        if (progress.status === "Completed" || progress.status === "Error") {
          // Ensure progress bar is filled completely when done
          progressFill.style.width = "100%";
          startButton.disabled = false;
          stopProgressPolling();
        }
      } catch (error) {
        console.error("Error polling progress:", error);
      }
    }, 1000);
  }

  function stopProgressPolling() {
    if (progressInterval) {
      clearInterval(progressInterval);
      progressInterval = null;
    }
  }

  // Initialize all categories as expanded
  document.querySelectorAll(".category-content").forEach((content) => {
    content.classList.remove("collapsed");
  });
  document.querySelectorAll(".category-header").forEach((header) => {
    header.classList.remove("collapsed");
  });

  // Set up input change listeners to save configuration
  const configInputs = document.querySelectorAll(
    "#uploadForm input, #uploadForm select"
  );
  configInputs.forEach((input) => {
    input.addEventListener("change", updateConfig);
  });
});
