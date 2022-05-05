const id = "drop-zone";

function setupDropZone() {
  console.log("Setting up");
  document.addEventListener("drop",      (event) => { metaHandler(event, dropHandler) });
  document.addEventListener("dragover",  (event) => { metaHandler(event, dragoverHandler) });
  document.addEventListener("dragleave", (event) => { metaHandler(event, disableHovering) });
}

function metaHandler(event, handler) {
  event.preventDefault();
  if (event.target.id == id) {
    handler(event);
  }
}

function dropHandler(event) {
  for (var i = 0; i < event.dataTransfer.files.length; i++) {
    console.log(event.dataTransfer.files[i])
  }

  disableHovering(event);
  if (event.dataTransfer.files.length < 0 || event.dataTransfer.files.length > 1) {
    // error
    return;
  }

  uploadFile(event.dataTransfer.files[0]);
}

function uploadFile(file) {
  const formData = new FormData();
  formData.append("file", event.dataTransfer.files[0])
  event.target.classList.add("uploading");
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

window.addEventListener("load", setupDropZone);
