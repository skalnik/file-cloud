const id = "drop-zone";
const maxAlbumFiles = 10;

function setupListeners() {
  document.addEventListener("drop",      (event) => { metaHandler(event, dropHandler) });
  document.addEventListener("dragover",  (event) => { metaHandler(event, dragoverHandler) });
  document.addEventListener("dragleave", (event) => { metaHandler(event, disableHovering) });

  document.getElementById("file-upload").addEventListener("change", (event) => {
    uploadFiles(event.target.files);
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
  uploadFiles(event.dataTransfer.files);
}

async function uploadFiles(files) {
  if (files.length < 1) {
    return;
  }

  if (files.length > maxAlbumFiles) {
    alert(`Albums are limited to ${maxAlbumFiles} files`);
    return;
  }

  const dropZone = document.getElementById(id);
  dropZone.setAttribute("aria-busy", true);

  const keys = [];
  try {
    for (const file of files) {
      keys.push(await uploadFile(file));
    }
  } catch (error) {
    dropZone.removeAttribute("aria-busy");
    alert(`Upload failed: ${error.message}`);
    return;
  }

  // A single key is just the normal file page; more form a stateless album
  window.location.href = `/${keys.join("+")}`;
}

async function uploadFile(file) {
  const formData = new FormData();
  formData.append("file", file);

  const response = await fetch("/", {
    method: "POST",
    body: formData,
  });

  if (!response.ok) {
    throw new Error(`server responded ${response.status}`);
  }

  const data = await response.json();
  return data.url.replace(/^\//, "");
}

function dragoverHandler(event) {
  event.target.classList.add("hover");
  event.dataTransfer.dropEffect = "copy";
}

function disableHovering(event) {
  event.target.classList.remove("hover");
}

window.addEventListener("load", setupListeners);
