(function () {
  const feed = document.querySelector(".news-feed");
  if (!feed) return;

  const posts = Array.from(feed.querySelectorAll(".news-post"));
  if (!posts.length) return;

  const getColumnCount = () => {
    const width = feed.clientWidth;
    if (width <= 720) return 1;
    if (width <= 900) return 2;
    if (width <= 1160) return 3;
    return 4;
  };

  const getGap = () => {
    const styles = window.getComputedStyle(feed);
    const rawGap = styles.columnGap || styles.gap || "18px";
    return Number.parseFloat(rawGap) || 18;
  };

  let frame = 0;
  const layout = () => {
    window.cancelAnimationFrame(frame);
    frame = window.requestAnimationFrame(() => {
      const columns = getColumnCount();
      const gap = getGap();
      const width = feed.clientWidth;
      const cardWidth = (width - gap * (columns - 1)) / columns;
      const heights = Array.from({ length: columns }, () => 0);

      feed.classList.add("is-masonry");

      posts.forEach((post, index) => {
        const column = index < columns
          ? index
          : heights.indexOf(Math.min(...heights));
        const x = Math.round(column * (cardWidth + gap));
        const y = Math.round(heights[column]);

        post.style.width = `${cardWidth}px`;
        post.style.transform = `translate(${x}px, ${y}px)`;
        heights[column] += post.offsetHeight + gap;
      });

      const feedHeight = Math.max(...heights) - gap;
      feed.style.height = `${Math.max(0, Math.round(feedHeight))}px`;
    });
  };

  posts.forEach((post) => {
    const image = post.querySelector("img");
    if (image && !image.complete) {
      image.addEventListener("load", layout, { once: true });
      image.addEventListener("error", layout, { once: true });
    }
  });

  if ("ResizeObserver" in window) {
    const observer = new ResizeObserver(layout);
    observer.observe(feed);
    posts.forEach((post) => observer.observe(post));
  } else {
    window.addEventListener("resize", layout);
  }

  layout();
})();
