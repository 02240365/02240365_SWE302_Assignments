// history.cy.js - Order History Service ("My orders").

describe('Order history', () => {
  beforeEach(() => {
    cy.openStore('/orders.html');
  });

  const showOrders = () => cy.get('[data-cy=orders-load-btn]').click();

  // ---- VALID ----
  it('says so when the customer has no orders', () => {
    cy.enterCustomer('C001');
    showOrders();
    cy.get('[data-cy=orders-empty]').should('contain', 'not placed any orders');
  });

  it('lists an order after it is placed', () => {
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P003', quantity: 2 }).then((order) => {
      cy.enterCustomer('C001');
      showOrders();
      cy.get('[data-cy=order-row]').should('have.length', 1);
      cy.get('[data-cy=order-row-id]').should('have.text', order.body.orderId);
      cy.get('[data-cy=order-row-total]').should('contain', 'Nu. 3520.00');
      cy.get('[data-cy=order-row-tracking]').should('have.text', order.body.trackingId);
    });
  });

  it('shows the newest order first', () => {
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P003', quantity: 1 });
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P002', quantity: 1 });
    cy.enterCustomer('C001');
    showOrders();
    cy.get('[data-cy=order-row]').should('have.length', 2);
    cy.get('[data-cy=order-row]').first().should('contain', 'Mechanical Keyboard');
  });

  it('records the real amounts paid (discount, coupon, shipping)', () => {
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P003', quantity: 2, couponCode: 'WELCOME10' }).then((order) => {
      // 3600 - 5% = 3420, coupon 342 -> 3078, + 100 shipping = 3178
      expect(order.body.finalTotal).to.eq(3178);
      cy.api('GET', `/api/orders/${order.body.orderId}`).then((res) => {
        expect(res.body.order.total).to.eq(3178);
        expect(res.body.order.discountTotal).to.eq(522); // 180 + 342
        expect(res.body.order.shippingFee).to.eq(100);
        expect(res.body.order.couponCode).to.eq('WELCOME10');
      });
    });
  });

  it('shows only the customer\'s own orders', () => {
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P003', quantity: 1 });
    cy.enterCustomer('C002');
    showOrders();
    cy.get('[data-cy=orders-empty]').should('exist');
  });

  // ---- INVALID ----
  it('asks for a Customer ID', () => {
    showOrders();
    cy.warningToast().should('contain', 'Please enter your Customer ID');
  });

  it('refuses an unknown customer or order ID (API)', () => {
    cy.api('GET', '/api/orders?customerId=C999').its('status').should('eq', 404);
    cy.api('GET', '/api/orders/ORD-DOES-NOT-EXIST').its('status').should('eq', 404);
  });

  it('does not record an order that failed', () => {
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P001', quantity: 11 }).its('status').should('eq', 409);
    cy.api('GET', '/api/orders?customerId=C001').its('body.orders').should('have.length', 0);
  });
});
