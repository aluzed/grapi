package naming

// The helpers below mirror the named methods of Strapi's Identifiers class.
// Some snake-case their input and some do not: that asymmetry is Strapi's and
// is kept, the names must match.

// ID, order and field column names.
const (
	IDColumn    = "id"
	OrderColumn = "order"
	FieldColumn = "field"
)

func (n Namer) one(name string, o Options) (string, error) { return n.Name([]string{name}, o) }

// Table is the table name of a collection.
func (n Namer) Table(collectionName string) (string, error) { return n.one(collectionName, Options{}) }

// JoinTable is <collection>_<attribute>_lnk.
func (n Namer) JoinTable(collectionName, attribute string) (string, error) {
	return n.Name([]string{collectionName, attribute}, Options{Suffix: "links"})
}

// MorphTable is <collection>_<attribute>_mph.
func (n Namer) MorphTable(collectionName, attribute string) (string, error) {
	return n.Name([]string{SnakeCase(collectionName), SnakeCase(attribute)}, Options{Suffix: "morphs"})
}

// Column is the column of an attribute.
func (n Namer) Column(attribute string) (string, error) { return n.one(attribute, Options{}) }

// JoinColumnID is <attribute>_id.
func (n Namer) JoinColumnID(attribute string) (string, error) {
	return n.one(attribute, Options{Suffix: "id"})
}

// InverseJoinColumnID is inv_<attribute>_id.
func (n Namer) InverseJoinColumnID(attribute string) (string, error) {
	return n.one(SnakeCase(attribute), Options{Prefix: "inv", Suffix: "id"})
}

// OrderColumnName is <singular>_ord.
func (n Namer) OrderColumnName(singular string) (string, error) {
	return n.one(singular, Options{Suffix: "order"})
}

// InverseOrderColumnName is inv_<singular>_ord.
func (n Namer) InverseOrderColumnName(singular string) (string, error) {
	return n.one(singular, Options{Prefix: "inv", Suffix: "order"})
}

// MorphJoinTableID is <singular>_id in a morph table.
func (n Namer) MorphJoinTableID(singular string) (string, error) {
	return n.one(SnakeCase(singular), Options{Suffix: "id"})
}

// MorphAttributeID is <attribute>_id of a morphTo relation.
func (n Namer) MorphAttributeID(attribute string) (string, error) {
	return n.one(SnakeCase(attribute), Options{Suffix: "id"})
}

// MorphType is <attribute>_type of a morphTo relation.
func (n Namer) MorphType(attribute string) (string, error) {
	return n.one(SnakeCase(attribute), Options{Suffix: "type"})
}

// Index names.
func (n Namer) Index(names ...string) (string, error) {
	return n.Name(names, Options{Suffix: "index"})
}

// FkIndex names a foreign key.
func (n Namer) FkIndex(names ...string) (string, error) { return n.Name(names, Options{Suffix: "fk"}) }

// UniqueIndex names a unique constraint.
func (n Namer) UniqueIndex(names ...string) (string, error) {
	return n.Name(names, Options{Suffix: "unique"})
}

// PrimaryIndex names a primary key.
func (n Namer) PrimaryIndex(names ...string) (string, error) {
	return n.Name(names, Options{Suffix: "primary"})
}

// InverseFkIndex names the inverse foreign key of a join table.
func (n Namer) InverseFkIndex(names ...string) (string, error) {
	return n.Name(names, Options{Suffix: "inv_fk"})
}

// OrderFkIndex names the order foreign key index of a join table.
func (n Namer) OrderFkIndex(names ...string) (string, error) {
	return n.Name(names, Options{Suffix: "order_fk"})
}

// OrderInverseFkIndex names the inverse order foreign key index.
func (n Namer) OrderInverseFkIndex(names ...string) (string, error) {
	return n.Name(names, Options{Suffix: "order_inv_fk"})
}

// IDColumnIndex names the id column index of a join table.
func (n Namer) IDColumnIndex(names ...string) (string, error) {
	return n.Name(names, Options{Suffix: "id_column_index"})
}

// OrderIndex names the order column index of a join table.
func (n Namer) OrderIndex(names ...string) (string, error) {
	return n.Name(names, Options{Suffix: "order_index"})
}
