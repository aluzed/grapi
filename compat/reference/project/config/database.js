module.exports = ({ env }) => {
  const client = env('DATABASE_CLIENT', 'sqlite');
  if (client === 'postgres') {
    return {
      connection: {
        client: 'postgres',
        connection: {
          host: env('DATABASE_HOST'),
          port: env.int('DATABASE_PORT', 5432),
          database: env('DATABASE_NAME'),
          user: env('DATABASE_USERNAME'),
          password: env('DATABASE_PASSWORD'),
        },
      },
    };
  }
  return {
    connection: {
      client: 'sqlite',
      connection: { filename: env('DATABASE_FILENAME', '/data/data.db') },
      useNullAsDefault: true,
    },
  };
};
