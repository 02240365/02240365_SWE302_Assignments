// coupon.cy.js - Coupon Service. A coupon is CHECKED before the order and
// only USED UP after the payment has gone through.

describe('Coupons', () => {
  beforeEach(() => {
    cy.openStore();
  });

  // ---- VALID ----
  it('accepts a valid coupon and shows what it is worth', () => {
    cy.enterCustomer('C001');
    cy.get('[data-cy=coupon-code]').type('WELCOME10');
    cy.get('[data-cy=coupon-check-btn]').click();
    cy.get('[data-cy=coupon-ok]').should('contain', 'WELCOME10').and('contain', 'Nu. 1000.00'); // 10% of 10,000
  });

  it('accepts the code in any letter case, with spaces', () => {
    cy.api('POST', '/api/coupons/validate', { customerId: 'C001', code: '  welcome10 ', subtotal: 2000 }).then((res) => {
      expect(res.status).to.eq(200);
      expect(res.body.code).to.eq('WELCOME10');
      expect(res.body.discountAmount).to.eq(200);
    });
  });

  it('takes the coupon off the order total', () => {
    cy.enterCustomer('C001');
    cy.get('[data-cy=coupon-code]').type('WELCOME10');
    cy.placeOrderUi('P003', 1); // 1800 - 180 = 1620, + 100 shipping
    cy.successToast().within(() => {
      cy.get('[data-cy=order-coupon]').should('contain', 'WELCOME10').and('contain', 'Nu. 180.00');
      cy.get('[data-cy=order-total]').should('contain', 'Nu. 1720.00');
    });
  });

  it('caps a percentage coupon at Nu. 5,000', () => {
    cy.api('POST', '/api/coupons/validate', { customerId: 'C001', code: 'WELCOME10', subtotal: 100000 })
      .its('body.discountAmount').should('eq', 5000);
  });

  it('gives a flat coupon its fixed amount', () => {
    cy.api('POST', '/api/coupons/validate', { customerId: 'C001', code: 'SAVE500', subtotal: 3000 })
      .its('body.discountAmount').should('eq', 500);
  });

  // ---- INVALID ----
  it('says when a coupon does not exist', () => {
    cy.fixture('coupons').then(({ unknown }) => {
      cy.enterCustomer('C001');
      cy.get('[data-cy=coupon-code]').type(unknown);
      cy.get('[data-cy=coupon-check-btn]').click();
      cy.get('[data-cy=coupon-error]').should('contain', 'does not exist');
    });
  });

  it('says when a coupon has expired', () => {
    cy.enterCustomer('C001');
    cy.get('[data-cy=coupon-code]').type('OLD20');
    cy.get('[data-cy=coupon-check-btn]').click();
    cy.get('[data-cy=coupon-error]').should('contain', 'expired');
  });

  it('asks for a code when the box is empty', () => {
    cy.enterCustomer('C001');
    cy.get('[data-cy=coupon-check-btn]').click();
    cy.get('[data-cy=coupon-error]').should('contain', 'please enter a coupon code');
  });

  it('refuses SAVE500 on an order below Nu. 3,000', () => {
    cy.api('POST', '/api/coupons/validate', { customerId: 'C001', code: 'SAVE500', subtotal: 2999 }).then((res) => {
      expect(res.status).to.eq(409);
      expect(res.body.message).to.contain('at least Nu. 3000');
    });
  });

  it('refuses an order that uses an expired coupon and places nothing', () => {
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P003', quantity: 1, couponCode: 'OLD20' }).then((res) => {
      expect(res.status).to.eq(409);
      expect(res.body.step).to.eq('coupon');
    });
    cy.api('GET', '/api/orders?customerId=C001').its('body.orders').should('have.length', 0);
  });

  // ---- ONE USE PER CUSTOMER ----
  it('lets a customer use a coupon only once', () => {
    const order = { customerId: 'C001', productId: 'P003', quantity: 1, couponCode: 'WELCOME10' };
    cy.api('POST', '/api/orders', order).its('status').should('eq', 200);
    cy.api('POST', '/api/orders', order).then((res) => {
      expect(res.status).to.eq(409);
      expect(res.body.message).to.contain('already been used');
    });
  });

  it('lets another customer use the same coupon', () => {
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P003', quantity: 1, couponCode: 'WELCOME10' });
    cy.api('POST', '/api/orders', { customerId: 'C002', productId: 'P003', quantity: 1, couponCode: 'WELCOME10' })
      .its('status').should('eq', 200);
  });

  it('does not use the coupon up when the order itself fails (regression D5)', () => {
    // 11 laptops: refused for stock AFTER the coupon was checked.
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P001', quantity: 11, couponCode: 'WELCOME10' })
      .its('status').should('eq', 409);
    // The coupon must still be usable for a real order.
    cy.api('POST', '/api/orders', { customerId: 'C001', productId: 'P003', quantity: 1, couponCode: 'WELCOME10' })
      .its('status').should('eq', 200);
  });
});
