LOG_FILE=$1

file_contents=$(cat "$LOG_FILE")
echo "$file_contents"

# This is just to make sure traefik started
if [[ "$file_contents" == *"Configuration loaded from file"* ]]; then
  echo "Traefik was started and loaded the correct configuration"
else
  echo "Traefik either didn't start or didn't load the correct configuration"
  exit 1
fi

# Verify that Traefik started and no errors are present related to the plugin
if [[ "$file_contents" == *"Plugins are disabled because an error has occurred."* ]]; then
  echo "Traefik started but there was an error loading the Traceable middleware plugin"
  exit 1
fi

if [[ "$file_contents" == *"invalid middleware"* ]]; then
  echo "Traefik started but there was an error loading the Traceable middleware plugin"
  exit 1
fi

if [[ "$file_contents" == *"error"* ]]; then
  echo "Traefik started but there were errors present in the logs"
  exit 1
fi

exit 0



