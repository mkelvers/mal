let searchTimeout;
const searchInput = document.getElementById('search-input');
const searchDropdown = document.getElementById('search-dropdown');

if (searchInput) {
  searchInput.addEventListener('input', function(e) {
    clearTimeout(searchTimeout);
    const query = e.target.value.trim();

    if (query.length < 2) {
      searchDropdown.innerHTML = '';
      return;
    }

    searchTimeout = setTimeout(() => {
      fetch('/api/search-quick?q=' + encodeURIComponent(query))
        .then(res => res.json())
        .then(results => {
          if (!results || results.length === 0) {
            searchDropdown.innerHTML = '';
            return;
          }

          let html = '<div class="search-results">';
          html += '<div class="search-results-title">Anime</div>';
          results.forEach(r => {
            html += `
              <a href="/anime/${r.id}" class="search-result-item">
                ${r.image ? `<img src="${r.image}" alt="${r.title}" class="search-result-thumb" />` : '<div class="search-result-no-image">no image</div>'}
                <div class="search-result-info">
                  <div class="search-result-title">${escapeHtml(r.title)}</div>
                  <div class="search-result-type">${r.type}</div>
                </div>
              </a>
            `;
          });
          html += `<a href="/search?q=${encodeURIComponent(query)}" class="search-result-view-all">View all results for ${escapeHtml(query)}</a>`;
          html += '</div>';
          searchDropdown.innerHTML = html;
        })
        .catch(err => console.error('Search error:', err));
    }, 300);
  });

  searchInput.addEventListener('blur', () => {
    setTimeout(() => {
      searchDropdown.innerHTML = '';
    }, 200);
  });

  document.addEventListener('click', (e) => {
    if (!e.target.closest('.header-search-wrapper')) {
      searchDropdown.innerHTML = '';
    }
  });
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}
