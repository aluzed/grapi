import type { Schema, Struct } from '@strapi/strapi';

export interface BlocksGallery extends Struct.ComponentSchema {
  collectionName: 'components_blocks_galleries';
  info: {
    displayName: 'Gallery';
  };
  attributes: {
    caption: Schema.Attribute.String;
    slides: Schema.Attribute.Integer & Schema.Attribute.DefaultTo<0>;
  };
}

export interface BlocksHero extends Struct.ComponentSchema {
  collectionName: 'components_blocks_heroes';
  info: {
    displayName: 'Hero';
  };
  attributes: {
    cta: Schema.Attribute.Component<'shared.link', false>;
    headline: Schema.Attribute.String & Schema.Attribute.Required;
  };
}

export interface BlocksRichText extends Struct.ComponentSchema {
  collectionName: 'components_blocks_rich_texts';
  info: {
    displayName: 'Rich text';
  };
  attributes: {
    body: Schema.Attribute.RichText;
  };
}

export interface LayoutCell extends Struct.ComponentSchema {
  collectionName: 'components_layout_cells';
  info: {
    displayName: 'Cell';
  };
  attributes: {
    authors: Schema.Attribute.Relation<'oneToMany', 'api::author.author'>;
    text: Schema.Attribute.String & Schema.Attribute.Required;
  };
}

export interface LayoutColumn extends Struct.ComponentSchema {
  collectionName: 'components_layout_columns';
  info: {
    displayName: 'Column';
  };
  attributes: {
    cell: Schema.Attribute.Component<'layout.cell', false>;
    width: Schema.Attribute.Integer &
      Schema.Attribute.SetMinMax<
        {
          max: 12;
          min: 1;
        },
        number
      >;
  };
}

export interface LayoutSection extends Struct.ComponentSchema {
  collectionName: 'components_layout_sections';
  info: {
    displayName: 'Section';
  };
  attributes: {
    columns: Schema.Attribute.Component<'layout.column', true>;
    heading: Schema.Attribute.String;
  };
}

export interface SharedLink extends Struct.ComponentSchema {
  collectionName: 'components_shared_links';
  info: {
    displayName: 'Link';
  };
  attributes: {
    author: Schema.Attribute.Relation<'oneToOne', 'api::author.author'>;
    label: Schema.Attribute.String & Schema.Attribute.Required;
    url: Schema.Attribute.String;
  };
}

export interface SharedSeo extends Struct.ComponentSchema {
  collectionName: 'components_shared_seos';
  info: {
    displayName: 'Seo';
  };
  attributes: {
    metaDescription: Schema.Attribute.Text;
    metaTitle: Schema.Attribute.String &
      Schema.Attribute.Required &
      Schema.Attribute.SetMinMaxLength<{
        maxLength: 60;
      }>;
    noIndex: Schema.Attribute.Boolean & Schema.Attribute.DefaultTo<false>;
  };
}

declare module '@strapi/strapi' {
  export namespace Public {
    export interface ComponentSchemas {
      'blocks.gallery': BlocksGallery;
      'blocks.hero': BlocksHero;
      'blocks.rich-text': BlocksRichText;
      'layout.cell': LayoutCell;
      'layout.column': LayoutColumn;
      'layout.section': LayoutSection;
      'shared.link': SharedLink;
      'shared.seo': SharedSeo;
    }
  }
}
