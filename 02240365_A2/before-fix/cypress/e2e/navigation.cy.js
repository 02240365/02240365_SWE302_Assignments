// navigation.cy.js - the multi-page site: landing page, menu, feature
// pages, remembered Customer ID. Each feature has its own page.

const PAGES = [
  { key: 'shop', path: '/shop.html', title: 'Shop' },
  { key: 'cart', path: '/cart.html', title: 'Shopping Cart' },
  { key: 'reviews', path: '/reviews.html', title: 'Product Reviews' },
  { key: 'orders', path: '/orders.html', title: 'My Orders and Tracking' },
  { key: 'status', path: '/status.html', title: 'System Status' },
];

describe('Site navigation and landing page', () => {
  beforeEach(() => {
    cy.openStore('/');
  });

  // ---- VALID ----
  it('shows the landing page with its hero, offers and featured products', () => {
    cy.get('[data-cy=hero-title]').should('be.visible');
    cy.get('[data-cy=featured-grid] a').should('have.length', 4);
    cy.get('[data-cy=offer-discount]').should('contain', '5% off from 2 units');
    cy.get('[data-cy=offer-shipping]').should('contain', 'Nu. 5,000');
    cy.get('[data-cy=offer-coupon]').should('contain', 'WELCOME10');
  });

  PAGES.forEach((p) => {
    it(`opens the ${p.title} page from the menu`, () => {
      cy.get(`[data-cy=nav-${p.key}]`).click();
      cy.location('pathname').should('eq', p.path);
      cy.get('[data-cy=page-title]').should('contain', p.title);
      cy.get(`[data-cy=nav-${p.key}]`).should('have.class', 'active');
    });
  });

  PAGES.forEach((p) => {
    it(`opens the ${p.title} page from its card on the landing page`, () => {
      cy.get(`[data-cy=home-card-${p.key}]`).click();
      cy.location('pathname').should('eq', p.path);
    });
  });

  it('returns to the landing page from the brand link', () => {
    cy.get('[data-cy=nav-shop]').click();
    cy.get('[data-cy=brand]').click();
    cy.location('pathname').should('eq', '/');
    cy.get('[data-cy=hero-title]').should('be.visible');
  });

  it('finds a product with the search on the landing page', () => {
    cy.get('[data-cy=hero-search-input]').type('P002');
    cy.get('[data-cy=hero-search-btn]').click();
    cy.location('pathname').should('eq', '/shop.html');
    cy.get('[data-cy=lookup-result]').should('contain', 'Mechanical Keyboard');
  });

  it('opens the shop from a featured product', () => {
    cy.get('[data-cy=featured-P003]').click();
    cy.get('[data-cy=lookup-result]').should('contain', 'Wireless Mouse');
  });

  it('remembers the Customer ID and coupon on every page', () => {
    cy.enterCustomer('C003');
    cy.get('[data-cy=coupon-code]').type('WELCOME10');
    PAGES.forEach((p) => {
      cy.get(`[data-cy=nav-${p.key}]`).click();
      cy.location('pathname').should('eq', p.path);
      cy.get('[data-cy=customer-id]').should('have.value', 'C003');
      cy.get('[data-cy=coupon-code]').should('have.value', 'WELCOME10');
    });
  });

  it('loads every page without a script error', () => {
    ['/', ...PAGES.map((p) => p.path)].forEach((path) => {
      cy.visit(path);
      cy.get('[data-cy=footer]').should('exist');
    });
  });

  // ---- INVALID ----
  it('searching with an empty box simply opens the shop', () => {
    cy.get('[data-cy=hero-search-btn]').click();
    cy.location('pathname').should('eq', '/shop.html');
    cy.location('search').should('eq', '');
  });

  it('shows a clear message for an unknown product searched from the landing page', () => {
    cy.get('[data-cy=hero-search-input]').type('P999');
    cy.get('[data-cy=hero-search-btn]').click();
    cy.get('[data-cy=lookup-result]').should('contain', 'does not exist');
  });

  it('answers 404 for a page that does not exist', () => {
    cy.request({ url: '/no-such-page.html', failOnStatusCode: false }).its('status').should('eq', 404);
  });
});
