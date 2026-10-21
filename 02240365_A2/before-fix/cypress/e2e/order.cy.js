// order.cy.js - placing an order from a product card. One order touches
// Customer, Product, Lambda/discount, Coupon, Shipping, Inventory, Payment,
// Order History and Notification, so these tests exercise the whole system.

describe('Placing an order', () => {
  beforeEach(() => {
    cy.openStore();
  });

  // ---- VALID ----
  it('places an order and shows the full receipt', () => {
    cy.enterCustomer('C001');
    cy.placeOrderUi('P003', 1);
    cy.successToast().within(() => {
      cy.get('[data-cy=order-id]').should('contain', 'ORD-C001-P003-');
      cy.contains('Sonam Zangmo');
      cy.get('[data-cy=order-total]').should('contain', 'Nu. 1900.00'); // 1800 + 100 shipping
      cy.get('[data-cy=order-shipping]').should('contain', 'Nu. 100.00').and('contain', 'WESTERN');
      cy.get('[data-cy=order-tracking]').should('contain', 'TRK-');
      cy.get('[data-cy=order-remaining]').should('contain', 'Remaining stock: 99');
    });
  });

  it('shows the three notifications from the Notification Service', () => {
    cy.enterCustomer('C001');
    cy.placeOrderUi('P003', 1);
    cy.successToast().find('.notification-list li').should('have.length', 3);
  });

  // The quantity discount tiers (Lambda rules): 5% from 2, 10% from 3, 15% from 5.
  it('applies the quantity discount tiers', () => {
    cy.fixture('products').then(({ discountTiers }) => {
      // The tiers are checked through the API so the four orders run quickly
      // and independently of toast timing.
      discountTiers.forEach((tier) => {
        cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P003', quantity: tier.qty }).then((res) => {
          expect(res.status, `quantity ${tier.qty}`).to.eq(200);
          expect(res.body.discountPercent || 0, `discount % for ${tier.qty}`).to.eq(tier.percent);
          expect(res.body.finalTotal, `total for ${tier.qty}`).to.eq(tier.total);
        });
      });
    });
  });

  it('shows the discount line on screen only when there is a discount', () => {
    cy.enterCustomer('C001');
    cy.placeOrderUi('P003', 2);
    cy.successToast().find('[data-cy=order-discount]').should('contain', '5%');
  });

  it('charges free shipping from Nu. 5,000 of goods, not below', () => {
    // 1 keyboard = Nu. 4,500 (below the limit) -> Nu. 100 shipping.
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P002', quantity: 1 }).then((res) => {
      expect(res.body.shippingFee).to.eq(100);
    });
    // 2 keyboards = 9,000 - 5% = Nu. 8,550 -> free.
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P002', quantity: 2 }).then((res) => {
      expect(res.body.shippingFee).to.eq(0);
      expect(res.body.finalTotal).to.eq(8550);
    });
  });

  it('shows the real remaining stock when the last units are bought (regression D3)', () => {
    cy.enterCustomer('C001');
    cy.placeOrderUi('P001', 10); // all 10 laptops
    cy.successToast().find('[data-cy=order-remaining]').should('contain', 'Remaining stock: 0');
  });

  it('gives two identical orders different order IDs (regression D2)', () => {
    const order = { customerId: 'C001', productId: 'P003', quantity: 1 };
    cy.api('POST', '/api/orders', order).then((first) => {
      cy.api('POST', '/api/orders', order).then((second) => {
        expect(first.body.orderId).to.be.a('string');
        expect(second.body.orderId).to.not.eq(first.body.orderId);
      });
    });
  });

  // ---- INVALID ----
  it('asks for a Customer ID before ordering', () => {
    cy.placeOrderUi('P003', 1);
    cy.warningToast().should('contain', 'Please enter your Customer ID');
  });

  it('refuses a quantity of zero', () => {
    cy.enterCustomer('C001');
    cy.placeOrderUi('P003', 0);
    cy.warningToast().should('contain', 'Quantity must be at least 1');
  });

  it('refuses an unknown customer', () => {
    cy.fixture('customers').then(({ unknownId }) => {
      cy.enterCustomer(unknownId);
      cy.placeOrderUi('P003', 1);
      cy.warningToast().should('contain', `Customer '${unknownId}' does not exist.`);
    });
  });

  it('refuses an unknown product (API)', () => {
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P999', quantity: 1 }).then((res) => {
      expect(res.status).to.eq(404);
      expect(res.body.message).to.eq("Product 'P999' does not exist.");
    });
  });

  it('refuses more units than are in stock, and does not use any up', () => {
    cy.enterCustomer('C001');
    cy.placeOrderUi('P001', 11); // only 10 in stock
    cy.warningToast().should('contain', 'insufficient stock');
    // The refused order must not have taken any stock: all 10 can still be bought.
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P001', quantity: 10 }).then((res) => {
      expect(res.status).to.eq(200);
      expect(res.body.remainingStock).to.eq(0);
    });
  });

  it('refuses a very large quantity (API)', () => {
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P003', quantity: 21 }).then((res) => {
      expect(res.status).to.eq(400);
      expect(res.body.message).to.contain('at most 20');
    });
  });

  it('refuses a negative quantity and a missing body (API)', () => {
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P003', quantity: -3 }).its('status').should('eq', 400);
    cy.api('POST', '/api/orders', { customerId: '', productId: '', quantity: 0 }).its('status').should('eq', 400);
  });

  it('refuses a body that is not JSON (API)', () => {
    cy.request({ method: 'POST', url: '/api/orders', body: '{not json', headers: { 'Content-Type': 'application/json' }, failOnStatusCode: false })
      .then((res) => {
        expect(res.status).to.eq(400);
        expect(res.body.message).to.eq('Malformed request body.');
      });
  });
});
