import type { Schema, Struct } from '@strapi/strapi';

export interface LongCategoryNameComponentWithALongName
  extends Struct.ComponentSchema {
  collectionName: 'components_long_category_name_component_with_a_long_names';
  info: {
    displayName: 'Component with a long name';
  };
  attributes: {
    anotherQuiteLongAttributeNameInsideTheComponent: Schema.Attribute.Integer;
    label: Schema.Attribute.String;
  };
}

declare module '@strapi/strapi' {
  export namespace Public {
    export interface ComponentSchemas {
      'long-category-name.component-with-a-long-name': LongCategoryNameComponentWithALongName;
    }
  }
}
