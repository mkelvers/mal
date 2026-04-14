// static/js/anime.ts
(() => {
  const toggleDropdown = () => {
    const dropdown = document.getElementById("watchlist-dropdown");
    if (!dropdown) {
      return;
    }
    dropdown.classList.toggle("open");
  };
  window.toggleDropdown = toggleDropdown;
  document.addEventListener("click", (event) => {
    const dropdown = document.getElementById("watchlist-dropdown");
    if (!dropdown) {
      return;
    }
    const target = event.target;
    if (!(target instanceof Node)) {
      return;
    }
    if (!dropdown.contains(target)) {
      dropdown.classList.remove("open");
    }
  });
})();
