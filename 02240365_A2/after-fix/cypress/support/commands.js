// Custom commands shared by all spec files (WEB303 Part 2).

// resetData: put every service back to its starting data (stock, carts,
// reviews, coupons used, orders, shipments). This is what keeps each test
// independent of the ones before it. The gateway only answers this call
// when it was started with ENABLE_TEST_ENDPOINTS=1 (./run-all.sh test).
Cypress.Commands.add('resetData', () => {
  cy.request({ method: 'POST', url: '/api/test/reset', failOnStatusCode: false }).then((res) => {
    expect(res.status, 'POST /api/test/reset (start the gateway with ./run-all.sh test)').to.eq(200);
  });
});

// openStore: reset the data, then open one page of the site and wait until
// it has drawn its main content (retry-ability replaces any fixed wait).
// Without arguments it opens the Shop page and waits for the catalogue.
const READY = {
  '/': '[data-cy=featured-P001]',
  '/shop.html': '[data-cy=product-card-P001]',
  '/cart.html': '[data-cy=cart-refresh-btn]',
  '/reviews.html': '[data-cy=review-average]',
  '/orders.html': '[data-cy=orders-load-btn]',
  '/status.html': '[data-cy=svc-product]',
};
Cypress.Commands.add('openStore', (path = '/shop.html') => {
  cy.resetData();
  cy.visit(path);
  cy.get(READY[path]).should('be.visible');
});

// openPage: go to another page of the site without resetting the data
// (the Customer ID typed earlier is remembered between pages).
Cypress.Commands.add('openPage', (path) => {
  cy.visit(path);
  cy.get(READY[path]).should('be.visible');
});

// enterCustomer: type a Customer ID into the header box.
Cypress.Commands.add('enterCustomer', (id) => {
  cy.get('[data-cy=customer-id]').clear().type(id);
});

// card: the product card for a product ID.
Cypress.Commands.add('card', (productId) => cy.get(`[data-cy=product-card-${productId}]`));

// setQty: set the quantity box on a product card.
Cypress.Commands.add('setQty', (productId, qty) => {
  cy.card(productId).find('[data-cy=qty-input]').clear().type(String(qty));
});

// placeOrderUi: click "Place Order" on a product card.
Cypress.Commands.add('placeOrderUi', (productId, qty) => {
  cy.setQty(productId, qty);
  cy.card(productId).find('[data-cy=place-order-btn]').click();
});

// addToCartUi: click "Add to cart" on a product card.
Cypress.Commands.add('addToCartUi', (productId, qty) => {
  cy.setQty(productId, qty);
  cy.card(productId).find('[data-cy=add-to-cart-btn]').click();
});

// successToast / warningToast: the toast panel for a successful / refused action.
Cypress.Commands.add('successToast', () => cy.get('[data-cy=toast][data-kind=success]', { timeout: 15000 }));
Cypress.Commands.add('warningToast', () => cy.get('[data-cy=toast][data-kind=warning]', { timeout: 15000 }));

// api: call the gateway directly. failOnStatusCode is off so that tests can
// check the error responses (400, 404, 409 ...) too.
Cypress.Commands.add('api', (method, url, body) =>
  cy.request({ method, url, body, failOnStatusCode: false, timeout: 30000 })
);
