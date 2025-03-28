document.addEventListener("DOMContentLoaded", function () {
  const dropZone = document.getElementById("drop-zone");
  const preview = document.getElementById("preview");
  const exportBtn = document.getElementById("export-btn");
  const frameWidthInput = document.getElementById("frameWidth");
  const frameHeightInput = document.getElementById("frameHeight");
  const dpiInput = document.getElementById("dpi");
  const rotateInput = document.getElementById("rotate");
  const keepAspectInput = document.getElementById("keepAspect");
  const cropImageInput = document.getElementById("cropImage"); // New crop checkbox

  // A scale factor for preview (1 inch becomes previewScale pixels)
  const previewScale = 50;

  // Array to store dropped files.
  let filesData = [];

  // Update previews based on current settings.
  function updatePreviews() {
    const frameWidth = parseFloat(frameWidthInput.value) || 8;
    const frameHeight = parseFloat(frameHeightInput.value) || 10;
    const rotate = rotateInput.checked;
    const keepAspect = keepAspectInput.checked;
    const crop = cropImageInput.checked;

    // Always use the given frame dimensions for the container.
    const effectiveWidth = frameWidth * previewScale;
    const effectiveHeight = frameHeight * previewScale;

    const containers = document.querySelectorAll(".image-container");
    containers.forEach((container) => {
      container.style.width = effectiveWidth + "px";
      container.style.height = effectiveHeight + "px";

      const img = container.querySelector("img");

      // Rotate only the image, and keep it centered.
      const rotationStr = rotate ? " rotate(90deg)" : "";
      img.style.transform = `translate(-50%, -50%)${rotationStr}`;

      if (crop) {
        img.style.objectFit = "cover";
      } else if (keepAspect) {
        img.style.objectFit = "contain";
      } else {
        img.style.objectFit = "fill";
      }
    });
  }

  // Listen for changes in settings.
  [
    frameWidthInput,
    frameHeightInput,
    dpiInput,
    rotateInput,
    keepAspectInput,
    cropImageInput,
  ].forEach((el) => {
    el.addEventListener("change", updatePreviews);
  });

  dropZone.addEventListener("dragover", function (e) {
    e.preventDefault();
    dropZone.classList.add("hover");
  });

  dropZone.addEventListener("dragleave", function (e) {
    e.preventDefault();
    dropZone.classList.remove("hover");
  });

  dropZone.addEventListener("drop", function (e) {
    e.preventDefault();
    dropZone.classList.remove("hover");
    const files = e.dataTransfer.files;
    for (let i = 0; i < files.length; i++) {
      if (files[i].type.startsWith("image/")) {
        filesData.push(files[i]);
        const reader = new FileReader();
        reader.onload = function (event) {
          const container = document.createElement("div");
          container.classList.add("image-container");

          const img = document.createElement("img");
          img.src = event.target.result;
          container.appendChild(img);
          preview.appendChild(container);
          updatePreviews();
        };
        reader.readAsDataURL(files[i]);
      }
    }
  });

  exportBtn.addEventListener("click", function () {
    const formData = new FormData();
    formData.append("frameWidth", frameWidthInput.value);
    formData.append("frameHeight", frameHeightInput.value);
    formData.append("dpi", dpiInput.value);
    if (rotateInput.checked) {
      formData.append("rotate", "on");
    }
    if (keepAspectInput.checked) {
      formData.append("keepAspect", "on");
    }
    if (cropImageInput.checked) {
      formData.append("crop", "on");
    }
    filesData.forEach((file) => {
      formData.append("images", file);
    });

    fetch("/generate", {
      method: "POST",
      body: formData,
    })
      .then((response) => response.blob())
      .then((blob) => {
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = "output.pdf";
        document.body.appendChild(a);
        a.click();
        a.remove();
      })
      .catch((error) => console.error("Error:", error));
  });
});
