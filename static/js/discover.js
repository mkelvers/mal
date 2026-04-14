// static/js/discover.ts
(() => {
  const setActiveTab = (clickedTab) => {
    const group = clickedTab.closest('[data-tab-group="discover"]');
    if (!group) {
      return;
    }
    const triggers = group.querySelectorAll("[data-tab-trigger]");
    triggers.forEach((tab) => tab.classList.remove("active"));
    clickedTab.classList.add("active");
  };
  document.addEventListener("click", (event) => {
    const target = event.target;
    if (!(target instanceof Element)) {
      return;
    }
    const trigger = target.closest("[data-tab-trigger]");
    if (!trigger) {
      return;
    }
    setActiveTab(trigger);
  });
})();
