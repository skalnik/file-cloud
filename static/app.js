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
  console.log('File(s) dropped');

  disableHovering(event);

  for (var i = 0; i < event.dataTransfer.files.length; i++) {
    console.log(`file[${i}].name = ${event.dataTransfer.files[i].name}`);
  }
}

function dragoverHandler(event) {
  event.target.classList.add("hover");
  event.dataTransfer.dropEffect = "copy";
}

function disableHovering(event) {
  event.target.classList.remove("hover");
}

window.addEventListener("load", setupDropZone);
