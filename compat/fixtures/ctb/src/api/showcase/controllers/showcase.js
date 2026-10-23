'use strict';

/**
 * showcase controller
 */

const { createCoreController } = require('@strapi/strapi').factories;

module.exports = createCoreController('api::showcase.showcase');
