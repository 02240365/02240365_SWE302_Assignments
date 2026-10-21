// system.cy.js - the ten microservices as a whole, plus how the gateway
// reacts to bad requests and to a service being down.

const SERVICES = ['product', 'inventory', 'customer', 'payment', 'notification', 'cart', 'review', 'shipping', 'coupon', 'history'];

describe('System status', () => {
  beforeEach(() => {
    cy.openStore('/status.html');
  });

  it('reports all ten services as up (API)', () => {
    cy.api('GET', '/api/services').then((res) => {
      expect(res.status).to.eq(200);
      expect(res.body.services).to.have.length(10);
      SERVICES.forEach((name) => {
        const s = res.body.services.find((x) => x.name === name);
        expect(s, name).to.exist;
        expect(s.up, `${name} is up`).to.eq(true);
      });
    });
  });

  it('shows all ten services as Up on the page', () => {
    cy.get('[data-cy=status-list] li').should('have.length', 10);
    SERVICES.forEach((name) => {
      cy.get(`[data-cy=svc-${name}]`).should('have.attr', 'data-up', 'true').and('contain', 'Up');
    });
  });

  it('uses the expected port for each service', () => {
    cy.api('GET', '/api/services').then((res) => {
      const ports = res.body.services.map((s) => s.port);
      expect(ports).to.deep.eq([50051, 50052, 50053, 50054, 50055, 50056, 50057, 50058, 50059, 50060]);
    });
  });

  it('shows a service as Down when it is down (simulated answer)', () => {
    // The browser is given an answer in which the coupon service is down,
    // so the test does not have to stop a real process.
    cy.intercept('GET', '/api/services', (req) => {
      req.continue((res) => {
        res.body.services.find((s) => s.name === 'coupon').up = false;
      });
    }).as('services');
    cy.get('[data-cy=status-refresh-btn]').click();
    cy.wait('@services');
    cy.get('[data-cy=svc-coupon]').should('have.attr', 'data-up', 'false').and('contain', 'Down');
    cy.get('[data-cy=svc-cart]').should('have.attr', 'data-up', 'true');
  });

  it('tells the shopper when the gateway cannot be reached', () => {
    cy.openPage('/shop.html');
    cy.intercept('POST', '/api/orders', { forceNetworkError: true }).as('order');
    cy.enterCustomer('C001');
    cy.placeOrderUi('P003', 1);
    cy.get('[data-cy=toast][data-kind=unavailable]').should('contain', 'Could not reach the Gateway');
  });

  it('shows a friendly message when a service answers "unavailable"', () => {
    cy.openPage('/shop.html');
    cy.intercept('POST', '/api/orders', {
      statusCode: 503,
      body: { success: false, step: 'payment', errorCode: 'unavailable', message: 'A dependent service is currently unavailable. Please try again shortly.' },
    });
    cy.enterCustomer('C001');
    cy.placeOrderUi('P003', 1);
    cy.get('[data-cy=toast][data-kind=unavailable]').should('contain', '[payment]').and('contain', 'currently unavailable');
  });
});

describe('Gateway robustness', () => {
  beforeEach(() => {
    cy.resetData();
  });

  it('answers a wrong HTTP method with 405 and a JSON message', () => {
    cy.api('PUT', '/api/orders', {}).then((res) => {
      expect(res.status).to.eq(405);
      expect(res.body.message).to.contain('Use POST');
    });
    cy.api('POST', '/api/shipping/quote').its('status').should('eq', 405);
    cy.api('GET', '/api/coupons/validate').its('status').should('eq', 405);
  });

  it('answers malformed JSON with 400 on every POST endpoint', () => {
    ['/api/cart', '/api/cart/checkout', '/api/reviews', '/api/coupons/validate'].forEach((url) => {
      cy.request({ method: 'POST', url, body: '{oops', headers: { 'Content-Type': 'application/json' }, failOnStatusCode: false })
        .its('status').should('eq', 400);
    });
  });

  it('refuses a request body larger than 64 KB', () => {
    cy.request({
      method: 'POST', url: '/api/reviews', headers: { 'Content-Type': 'application/json' }, failOnStatusCode: false,
      body: JSON.stringify({ productId: 'P001', customerId: 'C001', rating: 5, comment: 'x'.repeat(70000) }),
    }).its('status').should('eq', 400);
  });

  it('does not run an ID that looks like a path (API)', () => {
    cy.api('GET', '/api/products/..%2f..%2fetc%2fpasswd').then((res) => {
      expect(res.status).to.be.oneOf([400, 404]);
    });
  });

  it('copes with a very long customer ID', () => {
    cy.api('GET', `/api/cart?customerId=${'C'.repeat(5000)}`).its('status').should('eq', 404);
  });
});
