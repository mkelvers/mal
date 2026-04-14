// static/js/anime.ts
(() => {
  const parseClassList = (value) => {
    if (!value) {
      return [];
    }
    return value.split(" ").map((entry) => entry.trim()).filter((entry) => entry.length > 0);
  };
  const setMenuState = (menu, isOpen) => {
    const openClasses = parseClassList(menu.getAttribute("data-dropdown-open-classes"));
    const closedClasses = parseClassList(menu.getAttribute("data-dropdown-closed-classes"));
    if (isOpen) {
      menu.classList.remove(...closedClasses);
      menu.classList.add(...openClasses);
      return;
    }
    menu.classList.remove(...openClasses);
    menu.classList.add(...closedClasses);
  };
  const toggleDropdown = () => {
    const dropdown = document.getElementById("watchlist-dropdown");
    if (!dropdown) {
      return;
    }
    const isOpen = !dropdown.classList.contains("open");
    dropdown.classList.toggle("open", isOpen);
    const menu = dropdown.querySelector("[data-dropdown-menu]");
    if (menu instanceof HTMLElement) {
      setMenuState(menu, isOpen);
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
        setMenuState(menu, false);
      }
    }
  });
})();
