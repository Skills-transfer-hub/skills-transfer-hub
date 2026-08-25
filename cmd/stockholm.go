package cmd

const stockholmArt = `       _______
   .-"       "-.
  /  .-""""-.    \
 /  /  _  _  \    \
|  |  (o)(o)  |   |
|  |    __    |   |
|  | .-(__)-. |   |
 \  \ \____/ /   /
  \  '-.__.-'   /
   '-._______.-'
      / / \ \
     /_/   \_\`

func (a *app) runStockholm() error {
	a.writeStdout(stockholmArt)
	return nil
}
