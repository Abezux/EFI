const babel = require('@babel/core');

module.exports = {
  process(sourceText, sourcePath) {
    const result = babel.transformSync(sourceText, {
      filename: sourcePath,
      configFile: false,
      babelrc: false,
      presets: [
        [
          require.resolve('next/dist/compiled/babel/preset-react'),
          { runtime: 'automatic' },
        ],
        require.resolve('next/dist/compiled/babel/preset-typescript'),
      ],
      plugins: [
        require.resolve('next/dist/compiled/babel/plugin-transform-modules-commonjs'),
      ],
      sourceMaps: 'inline',
    });

    return {
      code: result ? result.code : sourceText,
    };
  },
};
