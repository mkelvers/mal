// static/js/anime.ts
(() => {
  const toggleDropdown = () => {
    const dropdown = document.getElementById("watchlist-dropdown");
    if (!dropdown) {
      return;
    }
    dropdown.classList.toggle("open");
    const menu = dropdown.querySelector("[data-dropdown-menu]");
    if (menu instanceof HTMLElement) {
      menu.classList.toggle("invisible");
      menu.classList.toggle("opacity-0");
    }
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
      const menu = dropdown.querySelector("[data-dropdown-menu]");
      if (menu instanceof HTMLElement) {
        menu.classList.add("invisible");
        menu.classList.add("opacity-0");
      }
    }
  });
})();
