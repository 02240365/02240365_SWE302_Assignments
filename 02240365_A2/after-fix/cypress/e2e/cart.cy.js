// cart.cy.js - Cart Service (add, merge, remove, empty, check out).

describe('Shopping cart', () => {
  beforeEach(() => {
    cy.openStore();
  });

  // The cart has its own page. The Customer ID typed on the Shop page is
  // remembered, so the cart opens already filled in.
  const goToCart = () => {
    cy.get('[data-cy=nav-cart]').click();
    cy.location('pathname').should('eq', '/cart.html');
  };
  const showCart = () => cy.get('[data-cy=cart-refresh-btn]').click();

  // ---- VALID ----
  it('adds a product and shows it in the cart with its total', () => {
    cy.enterCustomer('C001');
    cy.addToCartUi('P003', 2);
    cy.successToast().should('contain', 'Added to cart');
    goToCart();
    cy.get('[data-cy=cart-line-P003]').should('contain', 'Wireless Mouse').and('contain', 'Nu. 3600.00');
    cy.get('[data-cy=cart-subtotal]').should('contain', 'Nu. 3600.00');
    cy.get('[data-cy=cart-count]').should('contain', '2 item(s)');
  });

  it('merges the same product into one line', () => {
    cy.enterCustomer('C001');
    cy.addToCartUi('P003', 1);
    cy.get('[data-cy=toast]').should('have.length', 1);
    cy.addToCartUi('P003', 2);
    cy.get('[data-cy=toast]').should('have.length', 2);
    goToCart();
    cy.get('[data-cy=cart-line-P003] [data-cy=cart-qty]').should('have.text', '3');
    cy.get('[data-cy=cart-table] tbody tr').should('have.length', 1);
  });

  it('adds up several different products', () => {
    cy.enterCustomer('C001');
    cy.addToCartUi('P003', 1); // 1800
    cy.get('[data-cy=toast]').should('have.length', 1);
    cy.addToCartUi('P002', 1); // 4500
    cy.get('[data-cy=toast]').should('have.length', 2);
    goToCart();
    cy.get('[data-cy=cart-subtotal]').should('contain', 'Nu. 6300.00');
  });

  it('shows the shipping estimate for the customer address', () => {
    cy.enterCustomer('C002'); // Phuentsholing
    cy.addToCartUi('P003', 1);
    cy.successToast();
    goToCart();
    cy.get('[data-cy=cart-shipping]').should('contain', 'Phuentsholing').and('contain', 'Nu. 150.00');
  });

  it('removes one product', () => {
    cy.enterCustomer('C001');
    cy.addToCartUi('P003', 1);
    cy.get('[data-cy=toast]').should('have.length', 1);
    cy.addToCartUi('P002', 1);
    cy.get('[data-cy=toast]').should('have.length', 2);
    goToCart();
    cy.get('[data-cy=cart-line-P002]');
    cy.get('[data-cy=cart-line-P003] [data-cy=cart-remove-btn]').click();
    cy.get('[data-cy=cart-line-P003]').should('not.exist');
    cy.get('[data-cy=cart-line-P002]').should('exist');
  });

  it('empties the cart', () => {
    cy.enterCustomer('C001');
    cy.addToCartUi('P003', 1);
    cy.successToast();
    goToCart();
    cy.get('[data-cy=cart-clear-btn]').click();
    cy.get('[data-cy=cart-empty]').should('contain', 'Your cart is empty.');
  });

  it('keeps each customer\'s cart separate', () => {
    cy.api('POST', '/api/cart', { customerId: 'C001', productId: 'P003', quantity: 1 });
    cy.enterCustomer('C002');
    goToCart();
    showCart();
    cy.get('[data-cy=cart-empty]').should('exist');
  });

  it('checks out every line, empties the cart and records the orders', () => {
    cy.api('POST', '/api/cart', { customerId: 'C001', productId: 'P003', quantity: 1 });
    cy.api('POST', '/api/cart', { customerId: 'C001', productId: 'P002', quantity: 1 });
    cy.enterCustomer('C001');
    goToCart();
    cy.get('[data-cy=cart-checkout-btn]').click();
    // 1800 + 100 shipping, and 4500 + 100 shipping = 6500
    cy.successToast().find('[data-cy=checkout-title]').should('contain', '2 order(s)');
    cy.get('[data-cy=toast] [data-cy=checkout-total]').should('contain', 'Nu. 6500.00');
    cy.get('[data-cy=cart-empty]').should('exist');
    cy.api('GET', '/api/orders?customerId=C001').its('body.orders').should('have.length', 2);
  });

  // ---- INVALID ----
  it('asks for a Customer ID before adding', () => {
    cy.addToCartUi('P003', 1);
    cy.warningToast().should('contain', 'Please enter your Customer ID');
  });

  it('refuses more than 20 units of one product', () => {
    cy.api('POST', '/api/cart', { customerId: 'C001', productId: 'P003', quantity: 20 }).its('status').should('eq', 200);
    cy.api('POST', '/api/cart', { customerId: 'C001', productId: 'P003', quantity: 1 }).then((res) => {
      expect(res.status).to.eq(409);
      expect(res.body.message).to.contain('at most 20 units');
    });
  });

  it('refuses an unknown product, an unknown customer and a zero quantity (API)', () => {
    cy.api('POST', '/api/cart', { customerId: 'C001', productId: 'P999', quantity: 1 }).its('status').should('eq', 404);
    cy.api('POST', '/api/cart', { customerId: 'C999', productId: 'P003', quantity: 1 }).its('status').should('eq', 404);
    cy.api('POST', '/api/cart', { customerId: 'C001', productId: 'P003', quantity: 0 }).its('status').should('eq', 400);
  });

  it('cannot remove something that is not in the cart (API)', () => {
    cy.api('DELETE', '/api/cart/P003?customerId=C001').its('status').should('eq', 404);
  });

  it('cannot check out an empty cart (API)', () => {
    cy.api('POST', '/api/cart/checkout', { customerId: 'C001' }).then((res) => {
      expect(res.status).to.eq(409);
      expect(res.body.message).to.eq('Your cart is empty.');
    });
  });

  it('keeps the cart when checkout fails for lack of stock (API)', () => {
    cy.api('POST', '/api/cart', { customerId: 'C001', productId: 'P001', quantity: 15 });
    // The cart allows up to 20 of one product but only 10 laptops exist,
    // so checkout must be refused by the Inventory Service.
    cy.api('POST', '/api/cart/checkout', { customerId: 'C001' }).then((res) => {
      expect(res.status).to.eq(409);
      expect(res.body.step).to.eq('inventory');
    });
    cy.api('GET', '/api/cart?customerId=C001').its('body.itemCount').should('eq', 15);
  });
});
