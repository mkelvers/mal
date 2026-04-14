// static/js/discover.ts
(() => {
  const parseClassList = (value) => {
    if (!value) {
      return [];
    }
    return value.split(" ").map((entry) => entry.trim()).filter((entry) => entry.length > 0);
  };
  const setActiveTab = (clickedTab) => {
    const group = clickedTab.closest('[data-tab-group="discover"]');
    if (!group) {
      return;
    }
    const triggers = group.querySelectorAll("[data-tab-trigger]");
    triggers.forEach((tab) => {
      const activeClasses2 = parseClassList(tab.getAttribute("data-tab-active-classes"));
      const inactiveClasses2 = parseClassList(tab.getAttribute("data-tab-inactive-classes"));
      tab.classList.remove(...activeClasses2);
      tab.classList.add(...inactiveClasses2);
    });
    const activeClasses = parseClassList(clickedTab.getAttribute("data-tab-active-classes"));
    const inactiveClasses = parseClassList(clickedTab.getAttribute("data-tab-inactive-classes"));
    clickedTab.classList.remove(...inactiveClasses);
    clickedTab.classList.add(...activeClasses);
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
