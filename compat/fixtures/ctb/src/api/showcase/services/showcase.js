'use strict';

/**
 * showcase service
 */

const { createCoreService } = require('@strapi/strapi').factories;

module.exports = createCoreService('api::showcase.showcase');
