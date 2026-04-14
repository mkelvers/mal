// static/js/search.ts
(() => {
  const globalWindow = window;
  if (globalWindow.searchInitialized) {
    return;
  }
  globalWindow.searchInitialized = true;
  let searchTimeout;
  const searchInput = document.getElementById("search-input");
  const searchDropdown = document.querySelector("[data-search-results-container]");
  if (!searchInput || !searchDropdown) {
    return;
  }
  searchInput.addEventListener("input", (event) => {
    if (searchTimeout) {
      window.clearTimeout(searchTimeout);
    }
    const target = event.target;
    if (!(target instanceof HTMLInputElement)) {
      return;
    }
    const query = target.value.trim();
    if (query.length < 2) {
      searchDropdown.replaceChildren();
      return;
    }
    searchTimeout = window.setTimeout(() => {
      fetch("/api/search-quick?q=" + encodeURIComponent(query)).then((res) => res.json()).then((results) => {
        if (!results || results.length === 0) {
          searchDropdown.replaceChildren();
          return;
        }
        const searchResults = document.createElement("div");
        searchResults.className = "grid";
        const title = document.createElement("div");
        title.className = "px-3 py-2 text-[0.68rem] text-[var(--text-faint)]";
        title.textContent = "Anime";
        searchResults.appendChild(title);
        results.forEach((result) => {
          const item = document.createElement("a");
          item.className = "flex items-start gap-3 px-3 py-2 text-inherit no-underline hover:bg-[var(--panel-soft)] hover:no-underline";
          item.setAttribute("href", "/anime/" + encodeURIComponent(String(result.id || "")));
          if (isSafeImageUrl(result.image)) {
            const img = document.createElement("img");
            img.className = "aspect-[2/3] w-[42px] shrink-0 object-cover bg-[var(--surface-thumb)]";
            img.setAttribute("src", result.image || "");
            img.setAttribute("alt", String(result.title || ""));
            item.appendChild(img);
          } else {
            const noImage = document.createElement("div");
            noImage.className = "aspect-[2/3] w-[42px] shrink-0 bg-[var(--surface-thumb)] text-[0] text-transparent";
            noImage.textContent = "no image";
            item.appendChild(noImage);
          }
          const info = document.createElement("div");
          info.className = "grid min-w-0 gap-px";
          const itemTitle = document.createElement("div");
          itemTitle.className = "line-clamp-1 text-[0.86rem] leading-[1.3] text-[var(--text)]";
          itemTitle.textContent = String(result.title || "");
          info.appendChild(itemTitle);
          const itemType = document.createElement("div");
          itemType.className = "text-[0.67rem] text-[var(--text-faint)]";
          itemType.textContent = String(result.type || "");
          info.appendChild(itemType);
          item.appendChild(info);
          searchResults.appendChild(item);
        });
        const viewAll = document.createElement("a");
        viewAll.className = "bg-[var(--surface-search-view-all)] px-3 py-2 text-center text-[0.8rem] text-[var(--text-muted)] no-underline hover:bg-[var(--panel-soft)] hover:text-[var(--text)] hover:no-underline";
        viewAll.setAttribute("href", "/search?q=" + encodeURIComponent(query));
        viewAll.textContent = "View all results for " + query;
        searchResults.appendChild(viewAll);
        searchDropdown.replaceChildren(searchResults);
      }).catch((err) => {
        console.error("Search error:", err);
      });
    }, 300);
  });
  searchInput.addEventListener("blur", () => {
    window.setTimeout(() => {
      searchDropdown.replaceChildren();
    }, 200);
  });
  document.addEventListener("click", (event) => {
    const target = event.target;
    if (!(target instanceof Element)) {
      return;
    }
    if (!target.closest("[data-search-root]")) {
      searchDropdown.replaceChildren();
    }
  });
  function isSafeImageUrl(rawUrl) {
    if (!rawUrl || typeof rawUrl !== "string") {
      return false;
    }
    try {
      const parsed = new URL(rawUrl, window.location.origin);
      return parsed.protocol === "https:" || parsed.protocol === "http:";
    } catch {
      return false;
    }
  }
})();
