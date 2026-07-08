(function () {
  const version = "portal-favicon-v3";
  const frames = Array.from(
    { length: 7 },
    (_, index) => `/static/favicon-portal-frame-${String(index).padStart(2, "0")}.svg?v=${version}`,
  );
  const holdMs = 5000;
  const frameMs = 120;
  const toLight = [1, 0, 1, 2, 3, 4, 5, 6];
  const toDark = [5, 6, 5, 4, 3, 2, 1, 0];

  let currentFrame = 0;
  let timerId = 0;

  function iconLink() {
    let link = document.querySelector("link[data-tournament-favicon]");
    if (link) {
      return link;
    }

    link = document.createElement("link");
    link.rel = "icon";
    link.type = "image/svg+xml";
    link.setAttribute("data-tournament-favicon", "");
    document.head.appendChild(link);
    return link;
  }

  function setFrame(index) {
    currentFrame = index;
    iconLink().href = frames[index];
  }

  function schedule(callback, delay) {
    window.clearTimeout(timerId);
    timerId = window.setTimeout(callback, delay);
  }

  function play(sequence, index) {
    if (index >= sequence.length) {
      schedule(loop, holdMs);
      return;
    }

    setFrame(sequence[index]);
    schedule(() => play(sequence, index + 1), frameMs);
  }

  function loop() {
    play(currentFrame === 0 ? toLight : toDark, 0);
  }

  setFrame(currentFrame);
  schedule(loop, holdMs);
})();
