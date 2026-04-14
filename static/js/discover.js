// static/js/discover.ts
(() => {
  const setActiveTab = (clickedTab) => {
    const group = clickedTab.closest('[data-tab-group="discover"]');
    if (!group) {
      return;
    }
    const triggers = group.querySelectorAll("[data-tab-trigger]");
    triggers.forEach((tab) => {
      tab.classList.add("tab-trigger");
      tab.classList.remove("bg-[var(--surface-tab-active)]", "text-[var(--accent)]");
      tab.classList.add("bg-[var(--panel-soft)]", "text-[var(--text-muted)]");
    });
    clickedTab.classList.add("tab-trigger");
    clickedTab.classList.remove("bg-[var(--panel-soft)]", "text-[var(--text-muted)]");
    clickedTab.classList.add("bg-[var(--surface-tab-active)]", "text-[var(--accent)]");
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
