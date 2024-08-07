const id = "drop-zone";

function setupListeners() {
  document.addEventListener("drop",      (event) => { metaHandler(event, dropHandler) });
  document.addEventListener("dragover",  (event) => { metaHandler(event, dragoverHandler) });
  document.addEventListener("dragleave", (event) => { metaHandler(event, disableHovering) });

  document.getElementById("file-upload").addEventListener("change", (event) => {
    uploadFile(event.target.files[0]);
  });
}

function metaHandler(event, handler) {
  event.preventDefault();
  if (event.target.id == id) {
    handler(event);
  }
}

function dropHandler(event) {
  disableHovering(event);
  if (event.dataTransfer.files.length < 0 || event.dataTransfer.files.length > 1) {
    // error
    return;
  }

  uploadFile(event.dataTransfer.files[0]);
}

function uploadFile(file, busyElement) {
  const formData = new FormData();
  formData.append("file", file);
  document.getElementById(id).setAttribute('aria-busy', true);
  fetch("/", {
    method: "POST",
    body: formData,
  }).then(r => r.json())
    .then(data => {
      const url = data.url;
      window.location.href = url;
  });
}

function dragoverHandler(event) {
  event.target.classList.add("hover");
  event.dataTransfer.dropEffect = "copy";
}

function disableHovering(event) {
  event.target.classList.remove("hover");
}

window.addEventListener("load", setupListeners);
